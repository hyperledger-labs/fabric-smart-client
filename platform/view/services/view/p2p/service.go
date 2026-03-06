/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p

import (
	"context"
	"runtime/debug"

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
	// NewSessionContext returns a context for the given session.
	NewSessionContext(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error)
	// DeleteContext deletes the view context for the given context ID.
	DeleteContext(contextID string)
	// SetContext sets the root context.
	SetContext(ctx context.Context)
}

// CommLayer models the communication layer for P2P operations.
type CommLayer interface {
	// MasterSession returns the master session.
	MasterSession() (view.Session, error)
	// NewSessionWithID returns a new session for the given arguments.
	NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg interface{}) (view.Session, error)
}

// EndpointService models the dependency to the view-sdk's endpoint service.
// It provides methods to retrieve identities.
//
//go:generate counterfeiter -o mock/resolver.go -fake-name EndpointService . EndpointService
type EndpointService interface {
	// GetIdentity returns the identity for the given endpoint and public key ID.
	GetIdentity(endpoint string, pkID []byte) (view.Identity, error)
}

// Runner models a view runner.
type Runner interface {
	// RunView runs the given responder view in the given view context.
	RunView(viewCtx view.Context, responder view.View) (interface{}, error)
}

type defaultRunner struct{}

func (r *defaultRunner) RunView(viewCtx view.Context, responder view.View) (interface{}, error) {
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
	s.viewManager.SetContext(ctx)
	session, err := s.commLayer.MasterSession()
	if err != nil {
		return errors.Wrap(err, "failed getting master session")
	}
	go func() {
		for {
			ch := session.Receive()
			select {
			case msg := <-ch:
				go s.handleMessage(msg)
			case <-ctx.Done():
				logger.DebugfContext(ctx, "received done signal, stopping listening to messages on the master session")
				return
			}
		}
	}()
	return nil
}

// handleMessage handles an incoming message.
func (s *Service) handleMessage(msg *view.Message) {
	logger.Debugf("Will call responder view for context [%s]", msg.ContextID)
	responder, id, err := s.viewManager.ExistResponderForCaller(msg.Caller)
	if err != nil {
		logger.Errorf("[%s] No responder exists for [%s]: [%s]", s.identityProvider.DefaultIdentity(), msg.String(), err)
		return
	}
	if id.IsNone() {
		id = s.identityProvider.DefaultIdentity()
	}

	if err := s.respond(responder, id, msg); err != nil {
		logger.Errorf("[%s] error during respond [%s]", s.identityProvider.DefaultIdentity(), err)
	}
}

// respond executes a given responder view.
func (s *Service) respond(responder view.View, id view.Identity, msg *view.Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("respond triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("failed responding [%s]", r)
		}
	}()

	// get context
	viewCtx, isNew, err := s.getOrCreateContext(id, msg)
	if err != nil {
		return errors.WithMessagef(err, "failed getting context for [%s,%s]", msg.ContextID, id)
	}

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

		// try to send error back to caller
		if err = viewCtx.Session().SendError([]byte(err.Error())); err != nil {
			logger.Error(err.Error())
		}
	}

	return nil
}

// getOrCreateContext returns a view context for the given arguments.
func (s *Service) getOrCreateContext(id view.Identity, msg *view.Message) (view.Context, bool, error) {
	// get the caller identity
	callerIdentity, err := s.endpointService.GetIdentity(msg.FromEndpoint, msg.FromPKID)
	if err != nil {
		return nil, false, err
	}

	// create a new session with the ID we received
	backend, err := s.commLayer.NewSessionWithID(
		msg.SessionID,
		msg.ContextID,
		msg.FromEndpoint,
		msg.FromPKID,
		callerIdentity,
		msg,
	)
	if err != nil {
		return nil, false, err
	}

	return s.viewManager.NewSessionContext(
		msg.Ctx,
		msg.ContextID,
		backend,
		callerIdentity,
	)
}
