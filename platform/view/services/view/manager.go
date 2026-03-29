/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"reflect"
	"runtime/debug"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.opentelemetry.io/otel/trace"
)

const (
	SuccessLabel       tracing.LabelName = "success"
	ViewLabel          tracing.LabelName = "view"
	InitiatorViewLabel tracing.LabelName = "initiator_view"
)

var logger = logging.MustGetLogger()

// DisposableContext extends view.Context with additional functions for lifecycle management.
//
//go:generate counterfeiter -o mock/disposable_context.go -fake-name DisposableContext . DisposableContext
type DisposableContext interface {
	view.Context
	// Dispose releases all resources held by the context.
	Dispose()
	// PutSessionByID registers a session with the given ID and party in the context.
	PutSessionByID(viewID string, party view.Identity, session view.Session) error
}

// ContextFactory defines the interface for creating new view contexts.
//
//go:generate counterfeiter -o mock/context_factory.go -fake-name ContextFactory . ContextFactory
type ContextFactory interface {
	// NewForInitiator returns a new ParentContext for an initiator view.
	NewForInitiator(
		ctx context.Context,
		contextID string,
		id view.Identity,
		view view.View,
	) (ParentContext, error)

	// NewForResponder returns a new ParentContext for a responder view.
	NewForResponder(
		ctx context.Context,
		contextID string,
		me view.Identity,
		session view.Session,
		party view.Identity,
	) (ParentContext, error)
}

// Manager is responsible for managing view contexts and protocols.
type Manager struct {
	contextFactory   ContextFactory
	identityProvider IdentityProvider
	registry         *Registry
	metrics          *Metrics

	contexts   map[string]DisposableContext
	contextsMu sync.RWMutex
}

// NewManager returns a new instance of the view manager.
func NewManager(
	identityProvider IdentityProvider,
	registry *Registry,
	metrics *Metrics,
	contextFactory ContextFactory,
) *Manager {
	return &Manager{
		identityProvider: identityProvider,

		contexts: map[string]DisposableContext{},
		registry: registry,

		metrics:        metrics,
		contextFactory: contextFactory,
	}
}

// GetManager returns an instance of *Manager, if available, an error otherwise.
func GetManager(sp services.Provider) (*Manager, error) {
	s, err := sp.GetService(reflect.TypeOf((*Manager)(nil)))
	if err != nil {
		return nil, err
	}
	return s.(*Manager), nil
}

// RegisterFactory registers a view factory for the given ID.
func (cm *Manager) RegisterFactory(id string, factory Factory) error {
	return cm.registry.RegisterFactory(id, factory)
}

// NewView returns a new view instance for the given ID and input.
func (cm *Manager) NewView(id string, in []byte) (f view.View, err error) {
	return cm.registry.NewView(id, in)
}

// RegisterResponder registers a responder view for the given initiator view.
func (cm *Manager) RegisterResponder(responder view.View, initiatedBy interface{}) error {
	return cm.registry.RegisterResponder(responder, initiatedBy)
}

// RegisterResponderWithIdentity registers a responder view for the given initiator view and responder identity.
func (cm *Manager) RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy interface{}) error {
	return cm.registry.RegisterResponderWithIdentity(responder, id, initiatedBy)
}

// GetResponder returns the responder view for the given initiator view.
func (cm *Manager) GetResponder(initiatedBy interface{}) (view.View, error) {
	return cm.registry.GetResponder(initiatedBy)
}

// Initiate initiates a protocol for the given view ID.
func (cm *Manager) Initiate(ctx context.Context, id string) (interface{}, error) {
	v, err := cm.registry.GetView(id)
	if err != nil {
		return nil, err
	}

	return cm.InitiateViewWithIdentity(ctx, v, cm.identityProvider.DefaultIdentity())
}

// InitiateView initiates a protocol for the given view.
func (cm *Manager) InitiateView(ctx context.Context, view view.View) (interface{}, error) {
	return cm.InitiateViewWithIdentity(ctx, view, cm.identityProvider.DefaultIdentity())
}

// InitiateViewWithIdentity initiates a protocol for the given view and initiator identity.
func (cm *Manager) InitiateViewWithIdentity(ctx context.Context, view view.View, id view.Identity) (interface{}, error) {
	if ctx == nil {
		panic("context is nil")
	}
	ctx = trace.ContextWithSpanContext(ctx, trace.SpanContextFromContext(ctx))
	c, err := cm.newChildContextForInitiator(ctx, view, id, "")
	if err != nil {
		return nil, err
	}
	defer cm.DeleteContext(c.ID())

	logger.DebugfContext(ctx, "[%s] InitiateView [view:%s], [ContextID:%s], from [%s]", id, logging.Identifier(view), c.ID(), string(debug.Stack()))
	res, err := c.RunView(view)
	if err != nil {
		logger.DebugfContext(ctx, "[%s] InitiateView [view:%s], [ContextID:%s] failed [%s]", id, logging.Identifier(view), c.ID(), err)
		return nil, err
	}
	logger.DebugfContext(ctx, "[%s] InitiateView [view:%s], [ContextID:%s] terminated", id, logging.Identifier(view), c.ID())
	return res, nil
}

// InitiateContext initiates a view context for the given view.
func (cm *Manager) InitiateContext(ctx context.Context, view view.View) (view.Context, error) {
	return cm.InitiateContextFrom(ctx, view, cm.identityProvider.DefaultIdentity(), "")
}

// InitiateContextWithIdentity initiates a view context for the given view and initiator identity.
func (cm *Manager) InitiateContextWithIdentity(ctx context.Context, view view.View, id view.Identity) (view.Context, error) {
	return cm.InitiateContextFrom(ctx, view, id, "")
}

// InitiateContextWithIdentityAndID initiates a view context for the given view, initiator identity, and context ID.
func (cm *Manager) InitiateContextWithIdentityAndID(ctx context.Context, view view.View, id view.Identity, contextID string) (view.Context, error) {
	return cm.InitiateContextFrom(ctx, view, id, contextID)
}

// InitiateContextFrom initiates a view context for the given view, initiator identity, and context ID from the given go context.
func (cm *Manager) InitiateContextFrom(ctx context.Context, view view.View, id view.Identity, contextID string) (view.Context, error) {
	if id.IsNone() {
		id = cm.identityProvider.DefaultIdentity()
	}
	c, err := cm.newChildContextForInitiator(ctx, view, id, contextID)
	if err != nil {
		return nil, err
	}

	logger.DebugfContext(ctx, "[%s] InitiateContext [view:%s], [ContextID:%s]\n", id, logging.Identifier(view), c.ID())

	return c, nil
}

func (cm *Manager) newChildContextForInitiator(ctx context.Context, view view.View, id view.Identity, contextID string) (*ChildContext, error) {
	viewContext, err := cm.contextFactory.NewForInitiator(
		ctx,
		contextID,
		id,
		view,
	)
	if err != nil {
		return nil, err
	}
	c := NewChildContextFromParent(viewContext)
	cm.contextsMu.Lock()
	cm.contexts[c.ID()] = c
	cm.metrics.Contexts.Set(float64(len(cm.contexts)))
	cm.contextsMu.Unlock()

	context.AfterFunc(c.Context(), func() {
		cm.DeleteContext(c.ID())
	})

	return c, nil
}

// Context returns a view.Context for a given contextID. If the context does not exist, an error is returned.
func (cm *Manager) Context(contextID string) (view.Context, error) {
	cm.contextsMu.RLock()
	defer cm.contextsMu.RUnlock()
	viewCtx, ok := cm.contexts[contextID]
	if !ok {
		return nil, errors.Wrapf(ErrContextNotFound, "context %s not found", contextID)
	}
	return viewCtx, nil
}

// RegisterContext registers a view context.
func (cm *Manager) RegisterContext(contextID string, ctx DisposableContext) error {
	cm.contextsMu.Lock()
	defer cm.contextsMu.Unlock()
	cm.contexts[contextID] = ctx
	cm.metrics.Contexts.Set(float64(len(cm.contexts)))

	context.AfterFunc(ctx.Context(), func() {
		cm.DeleteContext(contextID)
	})

	return nil
}

// NewSessionContext returns a context for the given session.
// It returns the context, a boolean indicating if it's new, and an error.
func (cm *Manager) NewSessionContext(ctx context.Context, contextID string, session view.Session, me view.Identity, remote view.Identity) (view.Context, bool, error) {
	cm.contextsMu.Lock()
	defer cm.contextsMu.Unlock()

	if me.IsNone() {
		me = cm.identityProvider.DefaultIdentity()
	}

	sessionID := session.Info().ID
	caller := session.Info().Caller

	// check if a viewContext already exists for the given contextID
	viewContext, ok := cm.contexts[contextID]
	if ok && viewContext.Session() != nil && viewContext.Session().Info().ID != sessionID {
		// next we need to unwrap the actual context to store the session
		vCtx, ok := viewContext.(ParentContext)
		if !ok {
			panic("Not a ParentContext!")
		}

		// TODO: replace this with `vCtx.PutSession`, however, that method requires a view as input but we only have the viewID
		if err := vCtx.PutSessionByID(string(caller), remote, session); err != nil {
			return nil, false, errors.Wrapf(err, "failed registering session for [%s]", caller)
		}

		// we wrap our context and set our new session as the default session
		c := NewChildContextFromParentAndSession(vCtx, session)
		cm.contexts[contextID] = c
		cm.metrics.Contexts.Set(float64(len(cm.contexts)))

		return c, false, nil
	}
	if ok {
		logger.DebugfContext(viewContext.Context(), "[%s] No new context to respond, reuse [contextID:%s]\n", me, contextID)
		return viewContext, false, nil
	}

	// next we continue with creating a new context
	logger.Debugf("[%s] Create new context to respond [contextID:%s]\n", me, contextID)
	newCtx, err := cm.contextFactory.NewForResponder(
		ctx,
		contextID,
		me,
		session,
		remote,
	)
	if err != nil {
		return nil, false, err
	}

	c := NewChildContextFromParent(newCtx)
	cm.contexts[contextID] = c
	cm.metrics.Contexts.Set(float64(len(cm.contexts)))

	context.AfterFunc(c.Context(), func() {
		cm.DeleteContext(contextID)
	})

	return c, true, nil
}

// GetIdentifier returns the identifier for the given view.
func (cm *Manager) GetIdentifier(f view.View) string {
	return GetIdentifier(f)
}

// ExistResponderForCaller returns the responder view and identity for the given caller identifier.
func (cm *Manager) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
	return cm.registry.ExistResponderForCaller(caller)
}

// DeleteContext removes a context from the manager and calls Dispose on the context.
func (cm *Manager) DeleteContext(contextID string) {
	cm.contextsMu.Lock()
	defer cm.contextsMu.Unlock()

	// dispose context
	if viewCtx, ok := cm.contexts[contextID]; ok {
		viewCtx.Dispose()
		delete(cm.contexts, contextID)
		cm.metrics.Contexts.Set(float64(len(cm.contexts)))
	}
}
