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
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

// ViewManager models the view manager for P2P operations.
type ViewManager interface {
	// ExistResponderForCaller returns the responder view for the given caller.
	ExistResponderForCaller(caller string) (view2.View, view2.Identity, error)
	// GetIdentity returns the identity for the given endpoint and public key ID.
	GetIdentity(endpoint string, pkID []byte) (view2.Identity, error)
	// NewSessionContext returns a context for the given session.
	NewSessionContext(ctx context.Context, contextID string, session view2.Session, party view2.Identity) (view2.Context, bool, error)
	// DeleteContext deletes the view context for the given context ID.
	DeleteContext(id view2.Identity, contextID string)
	// Me returns the default identity.
	Me() view2.Identity
	// SetContext sets the root context.
	SetContext(ctx context.Context)
}

// CommLayer models the communication layer for P2P operations.
type CommLayer interface {
	// MasterSession returns the master session.
	MasterSession() (view2.Session, error)
	// NewSessionWithID returns a new session for the given arguments.
	NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view2.Identity, msg interface{}) (view2.Session, error)
}

// Service is responsible for handling incoming messages from the communication layer.
type Service struct {
	viewManager ViewManager
	commLayer   CommLayer
	runner      Runner
}

// Runner models a view runner.
type Runner interface {
	// RunView runs the given responder view in the given view context.
	RunView(viewCtx view2.Context, responder view2.View) (interface{}, error)
}

type defaultRunner struct{}

func (r *defaultRunner) RunView(viewCtx view2.Context, responder view2.View) (interface{}, error) {
	return viewCtx.RunView(responder)
}

// NewDefaultRunner returns a new instance of the default view runner.
func NewDefaultRunner() Runner {
	return &defaultRunner{}
}

// NewService returns a new instance of the P2P service.
func NewService(
	viewManager ViewManager,
	commLayer CommLayer,
	runner Runner,
) *Service {
	return &Service{
		viewManager: viewManager,
		commLayer:   commLayer,
		runner:      runner,
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
func (s *Service) handleMessage(msg *view2.Message) {
	logger.Debugf("Will call responder view for context [%s]", msg.ContextID)
	responder, id, err := s.viewManager.ExistResponderForCaller(msg.Caller)
	if err != nil {
		logger.Errorf("[%s] No responder exists for [%s]: [%s]", s.viewManager.Me(), msg.String(), err)
		return
	}
	if id.IsNone() {
		id = s.viewManager.Me()
	}

	if err := s.respond(msg.Ctx, responder, id, msg.ContextID, msg.SessionID, msg.Caller, msg.FromEndpoint, msg.FromPKID, msg); err != nil {
		logger.Errorf("[%s] error during respond [%s]", s.viewManager.Me(), err)
	}
}

// respond executes a given responder view.
func (s *Service) respond(ctx context.Context, responder view2.View, id view2.Identity, contextID, sessionID, caller, fromEndpoint string, fromPKID []byte, firstMsg interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("respond triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("failed responding [%s]", r)
		}
	}()

	// get context
	viewCtx, isNew, err := s.getOrCreateContext(ctx, id, contextID, sessionID, caller, fromEndpoint, fromPKID, firstMsg)
	if err != nil {
		return errors.WithMessagef(err, "failed getting context for [%s,%s]", contextID, id)
	}

	logger.DebugfContext(viewCtx.Context(), "[%s] Respond [from:%s], [sessionID:%s], [contextID:%s](%v), [view:%s]", id, fromEndpoint, sessionID, contextID, isNew, logging.Identifier(responder))

	// if a new context has been created to run the responder,
	// then dispose the context when not needed anymore
	if isNew {
		defer s.viewManager.DeleteContext(id, viewCtx.ID())
	}

	// run view
	_, err = s.runner.RunView(viewCtx, responder)
	if err != nil {
		logger.DebugfContext(viewCtx.Context(), "[%s] Respond Failure [from:%s], [sessionID:%s], [contextID:%s] [%s]\n", id, fromEndpoint, sessionID, contextID, err)

		// try to send error back to caller
		if err = viewCtx.Session().SendError([]byte(err.Error())); err != nil {
			logger.Error(err.Error())
		}
	}

	return nil
}

// getOrCreateContext returns a view context for the given arguments.
func (s *Service) getOrCreateContext(ctx context.Context, id view2.Identity, contextID, sessionID, caller, fromEndpoint string, fromPKID []byte, firstMsg interface{}) (view2.Context, bool, error) {
	// get the caller identity
	callerIdentity, err := s.viewManager.GetIdentity(fromEndpoint, fromPKID)
	if err != nil {
		return nil, false, err
	}

	// create a new session with the ID we received
	backend, err := s.commLayer.NewSessionWithID(sessionID, contextID, fromEndpoint, fromPKID, callerIdentity, firstMsg)
	if err != nil {
		return nil, false, err
	}

	return s.viewManager.NewSessionContext(ctx, contextID, backend, callerIdentity)
}
