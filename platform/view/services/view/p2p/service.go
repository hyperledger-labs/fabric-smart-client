/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
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

	// rejectTimeout bounds the error reply sent to a peer whose first message is rejected.
	rejectTimeout = 10 * time.Second
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
	// NewResponderContext returns a context used to respond to an invocation.
	NewResponderContext(ctx context.Context, contextID string, session view.Session, me, remote view.Identity) (view.Context, bool, error)
	// DeleteContext deletes the view context for the given context ID.
	DeleteContext(contextID string)
}

// CommLayer models the communication layer for P2P operations.
//
//go:generate counterfeiter -o mock/comm.go -fake-name CommLayer . CommLayer
type CommLayer interface {
	// MasterSession returns the master session.
	MasterSession() (view.Session, error)
	// NewResponderSession returns a new session for the given arguments.
	NewResponderSession(caller []byte, msg *view.Message) (view.Session, error)
	// ReplyError sends payload as an error to the sender of msg on msg's session, without
	// registering a session, so that no session in use by a responder is affected.
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
) *Service {
	maxRespondersPerPeer := DefaultMaxRespondersPerPeer
	if configService != nil && configService.IsSet(MaxRespondersPerPeerKey) {
		if v := configService.GetInt(MaxRespondersPerPeerKey); v > 0 {
			maxRespondersPerPeer = v
		}
	}
	return &Service{
		viewManager:          viewManager,
		identityProvider:     identityProvider,
		commLayer:            commLayer,
		endpointService:      endpointService,
		runner:               runner,
		maxRespondersPerPeer: maxRespondersPerPeer,
		inflight:             map[string]peerLoad{},
	}
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

// dispatch runs the responder for msg in a new goroutine, unless the sending peer
// (msg.FromPKID, set by the comm layer from the authenticated stream) already has
// maxRespondersPerPeer of them running. A message over the limit is rejected with an error
// reply, so that the initiator fails fast; while maxRespondersPerPeer rejections for that
// peer are pending, further messages are dropped. The two counts are independent, so
// pending rejections never keep a peer from starting responders. This caps what dispatch
// creates for one peer at maxRespondersPerPeer responders, each with its view context and
// comm session, plus maxRespondersPerPeer rejections. A slot is freed only when its
// goroutine returns; responders that never return keep their slot until shutdown. dispatch
// never blocks, so a single peer cannot stall the master session for the others.
func (s *Service) dispatch(ctx context.Context, msg *view.Message) {
	peer := string(msg.FromPKID)

	s.inflightMu.Lock()
	load := s.inflight[peer]
	admit := load.responders < s.maxRespondersPerPeer
	reject := !admit && load.rejections < s.maxRespondersPerPeer
	switch {
	case admit:
		load.responders++
	case reject:
		load.rejections++
	}
	s.inflight[peer] = load
	s.inflightMu.Unlock()

	if !admit && !reject {
		// Debug only: the peer controls how often this fires.
		logger.Debugf("dropping first message for context [%s] from [%s]: too many pending rejections", msg.ContextID, msg.FromEndpoint)
		return
	}
	s.wg.Go(func() {
		defer s.release(peer, admit)
		if admit {
			s.handleMessage(ctx, msg)
		} else {
			s.reject(ctx, msg)
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

// reject replies to msg with an error. The reply goes out through a one-shot session that
// is never registered, so it cannot disturb a responder that uses or is about to create a
// session with the same ID, as with a follow-up message that reached the master session
// before its responder registered the session. Rejections happen at a rate the peer
// controls, so reject logs at debug level only.
func (s *Service) reject(ctx context.Context, msg *view.Message) {
	logger.Debugf("rejecting first message for context [%s] from [%s]: [%d] responders running for this peer", msg.ContextID, msg.FromEndpoint, s.maxRespondersPerPeer)
	sendCtx, cancel := context.WithTimeout(ctx, rejectTimeout)
	defer cancel()
	if err := s.commLayer.ReplyError(sendCtx, msg, []byte("too many concurrent responders for this peer")); err != nil {
		logger.Debugf("failed rejecting context [%s]: [%s]", msg.ContextID, err)
	}
}

// handleMessage handles an incoming message. ctx is the Service's own lifecycle context
// (as passed to Start); it is threaded down to respond so that responder views have a
// best-effort way to observe shutdown while Start drains them via its WaitGroup.
func (s *Service) handleMessage(ctx context.Context, msg *view.Message) {
	logger.DebugfContext(ctx, "Will call responder view for context [%s]", msg.ContextID)
	responder, id, err := s.viewManager.ExistResponderForCaller(msg.Caller)
	if err != nil {
		logger.Errorf("[%s] No responder exists for [%s]: [%s]", s.identityProvider.DefaultIdentity(), msg.String(), err)
		return
	}
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
	viewCtx, isNew, cleanup, err := s.getOrCreateContext(ctx, id, msg)
	if err != nil {
		return errors.WithMessagef(err, "failed getting context for [%s,%s]", msg.ContextID, id)
	}
	// cleanup deregisters the AfterFunc callback and releases the merged context's
	// WithCancel resources once this responder is done (see getOrCreateContext).
	defer cleanup()

	logger.DebugfContext(viewCtx.Context(), "[%s] Respond [from:%s], [sessionID:%s], [contextID:%s](%v), [view:%s]", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, isNew, logging.Identifier(responder)) //nolint:contextcheck // deliberately logging against the responder's own view context (the merged ctx documented in getOrCreateContext), not this func's ctx param

	// if a new context has been created to run the responder,
	// then dispose the context when not needed anymore
	if isNew {
		defer s.viewManager.DeleteContext(viewCtx.ID())
	}

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
// context, to avoid leaking the AfterFunc registration and WithCancel resources created below.
func (s *Service) getOrCreateContext(ctx context.Context, me view.Identity, msg *view.Message) (viewCtx view.Context, isNew bool, cleanup func(), err error) {
	noop := func() {}

	// get the caller identity
	remote, err := s.endpointService.GetIdentity(msg.FromEndpoint, msg.FromPKID)
	if err != nil {
		return nil, false, noop, err
	}

	// create a new session with the ID we received
	responderSession, err := s.commLayer.NewResponderSession(remote, msg)
	if err != nil {
		return nil, false, noop, err
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
	cleanup = func() {
		// stop deregisters the AfterFunc callback (no-op if it already ran or msg.Ctx/ctx
		// were already done); cancel releases mergedCtx's WithCancel resources.
		stop()
		cancel()
	}

	viewCtx, isNew, err = s.viewManager.NewResponderContext( //nolint:contextcheck // documented above: mergedCtx deliberately merges msg.Ctx with this func's ctx (via context.AfterFunc), rather than passing ctx directly
		mergedCtx,
		msg.ContextID,
		responderSession,
		me,
		remote,
	)
	if err != nil {
		cleanup()
		return nil, false, noop, err
	}

	return viewCtx, isNew, cleanup, nil
}
