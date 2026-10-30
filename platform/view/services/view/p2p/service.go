/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p

import (
	"context"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

const (
	// MaxRespondersPerPeerKey is the configuration key that bounds the responders running
	// concurrently on behalf of one remote peer, and separately the rejections pending for it.
	MaxRespondersPerPeerKey = "fsc.p2p.maxRespondersPerPeer"
	// DefaultMaxRespondersPerPeer applies when MaxRespondersPerPeerKey is unset, zero or
	// negative. The bound cannot be disabled, so a mistyped value cannot turn it off.
	DefaultMaxRespondersPerPeer = 1000

	// maxRejectDetail bounds the detail of a rejection payload, in bytes.
	maxRejectDetail = 256
)

// rejectReason says why a first message is rejected. It prefixes the error payload and labels
// the rejections counter.
type rejectReason string

const (
	// rejectBusy rejects a message whose peer already runs maxRespondersPerPeer responders.
	rejectBusy rejectReason = "busy"
	// rejectUnknownView rejects a message whose caller view has no responder on this node.
	rejectUnknownView rejectReason = "unknown_view"
)

// ConfigService models the configuration service.
type ConfigService interface {
	// IsSet reports whether the given key is set.
	IsSet(key string) bool
	// GetInt returns the value of the given key as an int.
	GetInt(key string) int
}

// IdentityProvider models the identity provider for P2P operations.
type IdentityProvider interface {
	// DefaultIdentity returns the default identity.
	DefaultIdentity() view.Identity
}

// ViewManager models the view manager for P2P operations.
type ViewManager interface {
	// ExistResponderForCaller returns the responder view for the given caller.
	ExistResponderForCaller(caller string) (view.View, view.Identity, error)
	// NewResponderContext returns a context used to respond to an invocation, and the func that
	// releases it once the responder returns.
	NewResponderContext(ctx context.Context, contextID string, session view.Session, me, remote view.Identity) (view.Context, func(), error)
}

// CommLayer models the communication layer for P2P operations.
//
//go:generate counterfeiter -o mock/comm.go -fake-name CommLayer . CommLayer
type CommLayer interface {
	// MasterSession returns the master session.
	MasterSession() (view.Session, error)
	// NewResponderSession returns a new session for the given arguments.
	NewResponderSession(caller []byte, msg *view.Message) (view.Session, error)
	// ReplyError sends payload as an error with an empty caller to the sender of msg on msg's
	// session. It writes only to a stream that already exists and never opens a connection, and
	// it registers no session, so that no session in use by a responder is affected.
	ReplyError(ctx context.Context, msg *view.Message, payload []byte) error
}

// EndpointService models the dependency to the view-sdk's endpoint service.
// It provides methods to retrieve identities.
type EndpointService interface {
	// GetIdentity returns the identity for the given endpoint and public key ID.
	GetIdentity(endpoint string, pkID []byte) (view.Identity, error)
}

// Runner models a view runner.
type Runner interface {
	// RunView runs the given responder view in the given view context.
	RunView(viewCtx view.Context, responder view.View) (any, error)
}

type defaultRunner struct{}

func (*defaultRunner) RunView(viewCtx view.Context, responder view.View) (any, error) {
	return viewCtx.RunView(responder)
}

// NewDefaultRunner returns a new instance of the default view runner.
func NewDefaultRunner() Runner {
	return &defaultRunner{}
}

// Service is responsible for handling incoming messages from the communication layer.
type Service struct {
	viewManager      ViewManager
	identityProvider IdentityProvider
	endpointService  EndpointService
	commLayer        CommLayer
	runner           Runner

	// maxRespondersPerPeer bounds, per remote PKID, both the running responders and the
	// pending rejections.
	maxRespondersPerPeer int
	// inflight holds the goroutines dispatch runs per remote PKID.
	inflight   map[string]peerLoad
	inflightMu sync.Mutex

	// wg tracks the goroutines spawned by dispatch, so that shutdown (ctx.Done() or a
	// closed master session) can drain them before the Start goroutine returns.
	wg sync.WaitGroup

	// rejections counts the error replies sent by reject, by reason.
	rejections metrics.Counter
}

// Option configures a Service.
type Option func(*Service)

// WithMetrics makes the Service report its metrics to provider. Without it, metrics are
// discarded.
func WithMetrics(provider metrics.Provider) Option {
	return func(s *Service) {
		s.rejections = provider.NewCounter(metrics.CounterOpts{
			Name:       "rejections",
			Help:       "The number of error replies sent to first messages the node does not serve",
			LabelNames: []string{"reason"},
		})
	}
}

// NewService returns a new instance of the P2P service. The per-peer responder limit is
// read from MaxRespondersPerPeerKey; a nil configService selects the default.
func NewService(
	viewManager ViewManager,
	identityProvider IdentityProvider,
	commLayer CommLayer,
	endpointService EndpointService,
	runner Runner,
	configService ConfigService,
	opts ...Option,
) *Service {
	maxRespondersPerPeer := DefaultMaxRespondersPerPeer
	if configService != nil && configService.IsSet(MaxRespondersPerPeerKey) {
		if v := configService.GetInt(MaxRespondersPerPeerKey); v > 0 {
			maxRespondersPerPeer = v
		}
	}
	s := &Service{
		viewManager:          viewManager,
		identityProvider:     identityProvider,
		commLayer:            commLayer,
		endpointService:      endpointService,
		runner:               runner,
		maxRespondersPerPeer: maxRespondersPerPeer,
		inflight:             map[string]peerLoad{},
	}
	WithMetrics(&disabled.Provider{})(s)
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start starts the P2P service. It reads the master session, which receives every message
// whose session the node does not know yet, and dispatches each message to its responder
// view in a goroutine of its own, until ctx is done or the master session closes. In both
// cases Start waits for the dispatched goroutines to finish before it returns.
func (s *Service) Start(ctx context.Context) error {
	session, err := s.commLayer.MasterSession()
	if err != nil {
		return errors.Wrap(err, "failed getting master session")
	}
	ch := session.Receive()
	go func() {
		defer s.wg.Wait()
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					logger.ErrorfContext(ctx, "master session closed, no longer accepting incoming sessions")
					return
				}
				s.dispatch(ctx, msg)
			case <-ctx.Done():
				logger.DebugfContext(ctx, "received done signal, waiting for in-flight handlers")
				return
			}
		}
	}()
	return nil
}

// peerLoad counts the goroutines dispatch runs for one remote peer.
type peerLoad struct{ responders, rejections int }

// dispatch runs the responder for msg in a new goroutine, or rejects msg with an error
// reply, so that the initiator fails fast instead of waiting for its session to time out.
//
// A message with status view.ERROR or an empty caller is dropped: it cannot start a protocol.
// The error replies reject sends are such messages, so a rejection is never answered and two
// nodes cannot reject each other in a loop.
//
// A message whose caller view has no responder on this node is rejected as unknown_view. A late
// message from a responder whose initiator session on this node is gone usually has an empty
// caller and is dropped; it carries the initiator's view as caller, and is rejected, only if the
// initiator sent more than one message on that session. A
// message from a peer (msg.FromPKID, set by the comm layer from the authenticated stream)
// that already has maxRespondersPerPeer responders running is rejected as busy. While
// maxRespondersPerPeer rejections for that peer are pending, further rejections are dropped.
// The two counts are independent, so pending rejections never keep a peer from starting
// responders. This caps what dispatch creates for one peer at maxRespondersPerPeer
// responders, each with its view context and comm session, plus maxRespondersPerPeer
// rejections. A slot is freed only when its goroutine returns; responders that never return
// keep their slot until shutdown. A dropped message leaves no state behind. dispatch never
// blocks, so a single peer cannot stall the master session for the others.
func (s *Service) dispatch(ctx context.Context, msg *view.Message) {
	if msg.Status == view.ERROR || len(msg.Caller) == 0 {
		// Debug only: the peer controls how often this fires.
		logger.Debugf("dropping first message for context [%s] from [%s]: status [%d], caller [%s]", msg.ContextID, msg.FromEndpoint, msg.Status, msg.Caller)
		return
	}
	responder, id, err := s.viewManager.ExistResponderForCaller(msg.Caller)
	known := err == nil
	peer := string(msg.FromPKID)

	s.inflightMu.Lock()
	load := s.inflight[peer]
	run := known && load.responders < s.maxRespondersPerPeer
	reject := !run && load.rejections < s.maxRespondersPerPeer
	switch {
	case run:
		load.responders++
	case reject:
		load.rejections++
	default:
		s.inflightMu.Unlock()
		logger.Debugf("dropping first message for context [%s] from [%s]: too many pending rejections", msg.ContextID, msg.FromEndpoint)
		return
	}
	s.inflight[peer] = load
	s.inflightMu.Unlock()

	s.wg.Go(func() {
		defer s.release(peer, run)
		switch {
		case run:
			s.handleMessage(ctx, msg, responder, id)
		case known:
			s.reject(ctx, msg, rejectBusy, "too many concurrent responders for this peer")
		default:
			s.reject(ctx, msg, rejectUnknownView, msg.Caller)
		}
	})
}

// release frees the responder or rejection slot dispatch took for peer.
func (s *Service) release(peer string, responder bool) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	load := s.inflight[peer]
	if responder {
		load.responders--
	} else {
		load.rejections--
	}
	if load == (peerLoad{}) {
		delete(s.inflight, peer)
		return
	}
	s.inflight[peer] = load
}

// reject replies to msg with the error payload "<reason>: <detail>", detail sanitized by
// sanitizeDetail. The reply has an empty caller and goes out through the comm layer's
// ReplyError, which registers no session, so it cannot disturb a responder that uses or is
// about to create a session with the same ID, and which writes only to a stream that already
// exists, so a rejection never dials the peer. ctx does not bound the write: WebSocket bounds
// it with its write deadline, libp2p only by the stream's lifetime, so a peer that stops reading
// holds the rejection's slot until its stream resets. Rejections happen at a rate the peer
// controls, so reject logs at debug level only.
func (s *Service) reject(ctx context.Context, msg *view.Message, reason rejectReason, detail string) {
	logger.Debugf("rejecting first message for context [%s] from [%s]: %s", msg.ContextID, msg.FromEndpoint, reason)
	s.rejections.With("reason", string(reason)).Add(1)
	payload := string(reason) + ": " + sanitizeDetail(detail)
	if err := s.commLayer.ReplyError(ctx, msg, []byte(payload)); err != nil {
		logger.Debugf("failed rejecting context [%s]: [%s]", msg.ContextID, err)
	}
}

// sanitizeDetail returns detail truncated to maxRejectDetail bytes, with every rune that is not
// printable ASCII replaced by '?'. A detail may come from the peer, and the peer may log or
// display the payload it gets back.
func sanitizeDetail(detail string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r > '~' {
			return '?'
		}
		return r
	}, detail[:min(len(detail), maxRejectDetail)])
}

// handleMessage runs responder, which dispatch looked up for msg's caller, as id or, if id is
// none, as the default identity. ctx is the Service's own lifecycle context (as passed to
// Start); it is threaded down to respond so that responder views have a best-effort way to
// observe shutdown while Start drains them via its WaitGroup.
func (s *Service) handleMessage(ctx context.Context, msg *view.Message, responder view.View, id view.Identity) {
	logger.DebugfContext(ctx, "Will call responder view for context [%s]", msg.ContextID)
	if id.IsNone() {
		id = s.identityProvider.DefaultIdentity()
	}

	if err := s.respond(ctx, responder, id, msg); err != nil {
		logger.Errorf("[%s] error during respond [%s]", s.identityProvider.DefaultIdentity(), err)
	}
}

// respond executes a given responder view.
func (s *Service) respond(ctx context.Context, responder view.View, id view.Identity, msg *view.Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("respond triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("failed responding [%s]", r)
		}
	}()

	// get context
	viewCtx, cleanup, err := s.getOrCreateContext(ctx, id, msg)
	if err != nil {
		return errors.WithMessagef(err, "failed getting context for [%s,%s]", msg.ContextID, id)
	}
	// cleanup releases the view context once this responder is done (see getOrCreateContext).
	defer cleanup()

	logger.DebugfContext(viewCtx.Context(), "[%s] Respond [from:%s], [sessionID:%s], [contextID:%s], [view:%s]", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, logging.Identifier(responder)) //nolint:contextcheck // deliberately logging against the responder's own view context (the merged ctx documented in getOrCreateContext), not this func's ctx param

	// run view
	_, err = s.runner.RunView(viewCtx, responder)
	if err != nil {
		logger.DebugfContext(viewCtx.Context(), "[%s] Respond Failure [from:%s], [sessionID:%s], [contextID:%s] [%s]\n", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, err) //nolint:contextcheck // same as above: logging against the responder's own view context, not this func's ctx param

		// Keep error reporting uncancellable while preserving values from the responder context.
		if serr := viewCtx.Session().SendError(context.WithoutCancel(viewCtx.Context()), []byte(err.Error())); serr != nil { //nolint:contextcheck // documented above: error reporting is deliberately made uncancellable via WithoutCancel
			logger.Error(serr.Error())
		}
	}

	return nil
}

// getOrCreateContext returns a view context for the given arguments, along with a cleanup
// function the caller MUST invoke (typically via defer) once the responder is done with the
// context. cleanup releases the view context, which is disposed when its last responder
// releases it, and the AfterFunc registration and WithCancel resources created below.
func (s *Service) getOrCreateContext(ctx context.Context, me view.Identity, msg *view.Message) (viewCtx view.Context, cleanup func(), err error) {
	noop := func() {}

	// The caller is the peer ID the transport verified, not FromEndpoint: that is the
	// connection's source address, which the peer chooses.
	remote, err := s.endpointService.GetIdentity("", msg.FromPKID)
	if err != nil {
		return nil, noop, err
	}

	// create a new session with the ID we received
	responderSession, err := s.commLayer.NewResponderSession(remote, msg)
	if err != nil {
		return nil, noop, err
	}

	// The responder's view.Context must be cancelled when EITHER msg.Ctx is done (e.g. the
	// peer's stream closes) OR the Service's own lifecycle ctx is done (shutdown), while
	// still inheriting msg.Ctx's values (notably the incoming distributed-trace span, which
	// the websocket transport attaches via trace.ContextWithRemoteSpanContext). So derive
	// mergedCtx from msg.Ctx to preserve values + stream-close cancellation, and additionally
	// cancel it when ctx (Start's ctx) fires, so Start's wg.Wait() drain can never hang on a
	// transport (e.g. libp2p) whose stream context never cancels on its own.
	mergedCtx, cancel := context.WithCancel(msg.Ctx)
	stop := context.AfterFunc(ctx, cancel)
	viewCtx, release, err := s.viewManager.NewResponderContext( //nolint:contextcheck // documented above: mergedCtx deliberately merges msg.Ctx with this func's ctx (via context.AfterFunc), rather than passing ctx directly
		mergedCtx,
		msg.ContextID,
		responderSession,
		me,
		remote,
	)
	if err != nil {
		stop()
		cancel()
		// No context holds the responder session, so no Dispose would ever close it.
		// responderSession may be a live session: a follow-up message on a new session can reach
		// the master session before its responder registered the session. Such a message carries
		// the same session ID and ContextID, so NewResponderContext takes the same-session reuse
		// path and does not fail. Closing therefore affects only sessions keyed by the sender's
		// own PKID that it reused with a ContextID it may not join.
		responderSession.Close()
		return nil, noop, err
	}

	return viewCtx, func() {
		release()
		// stop deregisters the AfterFunc callback (no-op if it already ran or msg.Ctx/ctx
		// were already done); cancel releases mergedCtx's WithCancel resources.
		stop()
		cancel()
	}, nil
}
