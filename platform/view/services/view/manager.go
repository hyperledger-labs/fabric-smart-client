/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
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

//go:generate counterfeiter -o mock/view_manager.go -fake-name ViewManager . ViewManager

// ViewManager is responsible for managing view contexts and protocols.
type ViewManager interface {
	// NewView returns a new view instance for the given ID and input.
	NewView(id string, in []byte) (view.View, error)
	// InitiateView initiates a protocol for the given view and returns the result.
	InitiateView(view view.View, ctx context.Context) (interface{}, error)
	// InitiateContext initiates a view context for the given view.
	InitiateContext(view view.View) (view.Context, error)
	// DeleteContext removes a context from the manager.
	DeleteContext(id view.Identity, contextID string)
	// Me returns the default identity.
	Me() view.Identity
}

//go:generate counterfeiter -o mock/disposable_context.go -fake-name DisposableContext . DisposableContext

// DisposableContext extends view.Context with additional functions for lifecycle management.
type DisposableContext interface {
	view.Context
	// Dispose releases all resources held by the context.
	Dispose()
	// PutSessionByID registers a session with the given ID and party in the context.
	PutSessionByID(viewID string, party view.Identity, session view.Session) error
}

// Manager is responsible for managing view contexts and protocols.
type Manager struct {
	serviceProvider services.Provider

	sessionFactory       SessionFactory
	endpointService      EndpointService
	identityProvider     IdentityProvider
	registry             *Registry
	tracer               trace.Tracer
	metrics              *Metrics
	localIdentityChecker LocalIdentityChecker

	ctx        context.Context
	contexts   map[string]DisposableContext
	contextsMu sync.RWMutex
}

// NewManager returns a new instance of the view manager.
func NewManager(
	serviceProvider services.Provider,
	sessionFactory SessionFactory,
	endpointService EndpointService,
	identityProvider IdentityProvider,
	registry *Registry,
	tracerProvider tracing.Provider,
	metricsProvider metrics.Provider,
	localIdentityChecker LocalIdentityChecker,
) *Manager {
	return &Manager{
		serviceProvider:  serviceProvider,
		sessionFactory:   sessionFactory,
		endpointService:  endpointService,
		identityProvider: identityProvider,

		contexts: map[string]DisposableContext{},
		registry: registry,

		tracer: tracerProvider.Tracer("calls", tracing.WithMetricsOpts(tracing.MetricsOpts{
			LabelNames: []string{SuccessLabel, ViewLabel, InitiatorViewLabel},
		})),
		metrics:              newMetrics(metricsProvider),
		localIdentityChecker: localIdentityChecker,
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

// GetService returns the service of the given type from the underlying service provider.
func (cm *Manager) GetService(typ reflect.Type) (interface{}, error) {
	return cm.serviceProvider.GetService(typ)
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
func (cm *Manager) Initiate(id string, ctx context.Context) (interface{}, error) {
	v, err := cm.registry.GetView(id)
	if err != nil {
		return nil, err
	}

	return cm.InitiateViewWithIdentity(v, cm.Me(), ctx)
}

// InitiateView initiates a protocol for the given view.
func (cm *Manager) InitiateView(view view.View, ctx context.Context) (interface{}, error) {
	return cm.InitiateViewWithIdentity(view, cm.Me(), ctx)
}

// InitiateViewWithIdentity initiates a protocol for the given view and initiator identity.
func (cm *Manager) InitiateViewWithIdentity(view view.View, id view.Identity, ctx context.Context) (interface{}, error) {
	if ctx == nil {
		ctx = cm.getCurrentContext()
	}
	ctx = trace.ContextWithSpanContext(ctx, trace.SpanContextFromContext(ctx))
	c, err := cm.newChildContextForInitiator(ctx, view, id, "")
	if err != nil {
		return nil, err
	}
	defer cm.DeleteContext(id, c.ID())

	logger.DebugfContext(ctx, "[%s] InitiateView [view:%s], [ContextID:%s]", id, logging.Identifier(view), c.ID())
	res, err := c.RunView(view)
	if err != nil {
		logger.DebugfContext(ctx, "[%s] InitiateView [view:%s], [ContextID:%s] failed [%s]", id, logging.Identifier(view), c.ID(), err)
		return nil, err
	}
	logger.DebugfContext(ctx, "[%s] InitiateView [view:%s], [ContextID:%s] terminated", id, logging.Identifier(view), c.ID())
	return res, nil
}

// InitiateContext initiates a view context for the given view.
func (cm *Manager) InitiateContext(view view.View) (view.Context, error) {
	return cm.InitiateContextFrom(cm.getCurrentContext(), view, cm.Me(), "")
}

// InitiateContextWithIdentity initiates a view context for the given view and initiator identity.
func (cm *Manager) InitiateContextWithIdentity(view view.View, id view.Identity) (view.Context, error) {
	return cm.InitiateContextFrom(cm.getCurrentContext(), view, id, "")
}

// InitiateContextWithIdentityAndID initiates a view context for the given view, initiator identity, and context ID.
func (cm *Manager) InitiateContextWithIdentityAndID(view view.View, id view.Identity, contextID string) (view.Context, error) {
	return cm.InitiateContextFrom(cm.getCurrentContext(), view, id, contextID)
}

// InitiateContextFrom initiates a view context for the given view, initiator identity, and context ID from the given go context.
func (cm *Manager) InitiateContextFrom(ctx context.Context, view view.View, id view.Identity, contextID string) (view.Context, error) {
	if id.IsNone() {
		id = cm.Me()
	}
	c, err := cm.newChildContextForInitiator(ctx, view, id, contextID)
	if err != nil {
		return nil, err
	}

	logger.DebugfContext(ctx, "[%s] InitiateContext [view:%s], [ContextID:%s]\n", id, logging.Identifier(view), c.ID())

	return c, nil
}

func (cm *Manager) newChildContextForInitiator(ctx context.Context, view view.View, id view.Identity, contextID string) (*ChildContext, error) {
	viewContext, err := NewContextForInitiator(
		contextID,
		ctx,
		cm.serviceProvider,
		cm.sessionFactory,
		cm.endpointService,
		cm.identityProvider,
		id,
		view,
		cm.tracer,
		cm.localIdentityChecker,
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
		cm.DeleteContext(id, c.ID())
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
		cm.DeleteContext(ctx.Me(), contextID)
	})

	return nil
}

// NewSessionContext returns a context for the given session.
// It returns the context, a boolean indicating if it's new, and an error.
func (cm *Manager) NewSessionContext(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error) {
	cm.contextsMu.Lock()
	defer cm.contextsMu.Unlock()

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
		if err := vCtx.PutSessionByID(string(caller), party, session); err != nil {
			return nil, false, errors.Wrapf(err, "failed registering session for [%s]", caller)
		}

		// we wrap our context and set our new session as the default session
		c := NewChildContextFromParentAndSession(vCtx, session)
		cm.contexts[contextID] = c
		cm.metrics.Contexts.Set(float64(len(cm.contexts)))

		return c, false, nil
	}
	if ok {
		logger.DebugfContext(viewContext.Context(), "[%s] No new context to respond, reuse [contextID:%s]\n", cm.Me(), contextID)
		return viewContext, false, nil
	}

	// next we continue with creating a new context
	logger.Debugf("[%s] Create new context to respond [contextID:%s]\n", cm.Me(), contextID)
	newCtx, err := NewContext(
		ctx,
		cm.serviceProvider,
		contextID,
		cm.sessionFactory,
		cm.endpointService,
		cm.identityProvider,
		cm.Me(),
		session,
		party,
		cm.tracer,
		cm.localIdentityChecker,
	)
	if err != nil {
		return nil, false, err
	}

	c := NewChildContextFromParent(newCtx)
	cm.contexts[contextID] = c
	cm.metrics.Contexts.Set(float64(len(cm.contexts)))

	context.AfterFunc(c.Context(), func() {
		cm.DeleteContext(cm.Me(), contextID)
	})

	return c, true, nil
}

// GetIdentity returns the identity for the given endpoint and public key ID.
func (cm *Manager) GetIdentity(endpoint string, pkID []byte) (view.Identity, error) {
	return cm.endpointService.GetIdentity(endpoint, pkID)
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
func (cm *Manager) DeleteContext(id view.Identity, contextID string) {
	cm.contextsMu.Lock()
	defer cm.contextsMu.Unlock()

	logger.Debugf("[%s] Delete context [contextID:%s]\n", id, contextID)
	// dispose context
	if viewCtx, ok := cm.contexts[contextID]; ok {
		viewCtx.Dispose()
		delete(cm.contexts, contextID)
		cm.metrics.Contexts.Set(float64(len(cm.contexts)))
	}
}

// Me returns the default identity.
func (cm *Manager) Me() view.Identity {
	return cm.identityProvider.DefaultIdentity()
}

func (cm *Manager) getCurrentContext() context.Context {
	cm.contextsMu.Lock()
	ctx := cm.ctx
	cm.contextsMu.Unlock()
	return ctx
}

// SetContext sets the root context.
func (cm *Manager) SetContext(ctx context.Context) {
	cm.contextsMu.Lock()
	cm.ctx = ctx
	cm.contextsMu.Unlock()
}
