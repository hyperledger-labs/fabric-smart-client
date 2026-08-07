/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

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

func (r *defaultRunner) RunView(viewCtx view.Context, responder view.View) (any, error) {
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

	// wg tracks in-flight handleMessage goroutines spawned by Start, so that shutdown
	// (ctx.Done()) can drain them before the Start goroutine returns (Issue #7).
	wg sync.WaitGroup
}

// NewService returns a new instance of the P2P service.
func NewService(
	viewManager ViewManager,
	identityProvider IdentityProvider,
	commLayer CommLayer,
	endpointService EndpointService,
	runner Runner,
) *Service {
	return &Service{
		viewManager:      viewManager,
		identityProvider: identityProvider,
		commLayer:        commLayer,
		endpointService:  endpointService,
		runner:           runner,
	}
}

// Start starts the P2P service.
func (s *Service) Start(ctx context.Context) error {
	session, err := s.commLayer.MasterSession()
	if err != nil {
		return errors.Wrap(err, "failed getting master session")
	}
	go func() {
		for {
			ch := session.Receive()
			select {
			case msg := <-ch:
				s.wg.Go(func() {
					s.handleMessage(ctx, msg)
				})
			case <-ctx.Done():
				logger.DebugfContext(ctx, "received done signal, waiting for in-flight handlers")
				s.wg.Wait()
				return
			}
		}
	}()
	return nil
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

	logger.DebugfContext(viewCtx.Context(), "[%s] Respond [from:%s], [sessionID:%s], [contextID:%s](%v), [view:%s]", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, isNew, logging.Identifier(responder))

	// if a new context has been created to run the responder,
	// then dispose the context when not needed anymore
	if isNew {
		defer s.viewManager.DeleteContext(viewCtx.ID())
	}

	// run view
	_, err = s.runner.RunView(viewCtx, responder)
	if err != nil {
		logger.DebugfContext(viewCtx.Context(), "[%s] Respond Failure [from:%s], [sessionID:%s], [contextID:%s] [%s]\n", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, err)

		// send the error back to the caller
		if serr := viewCtx.Session().SendError([]byte(err.Error())); serr != nil {
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

	viewCtx, isNew, err = s.viewManager.NewResponderContext(
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
