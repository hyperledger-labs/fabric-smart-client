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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
)

const (
	SuccessLabel       tracing.LabelName = "success"
	ViewLabel          tracing.LabelName = "view"
	InitiatorViewLabel tracing.LabelName = "initiator_view"
)

var logger = logging.MustGetLogger()

// DisposableContext extends view.Context with additional functions
type DisposableContext interface {
	view.Context
	Dispose()
}

type Manager struct {
	serviceProvider services.Provider

	commLayer            CommLayer
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

func NewManager(
	serviceProvider services.Provider,
	commLayer CommLayer,
	endpointService EndpointService,
	identityProvider IdentityProvider,
	registry *Registry,
	tracerProvider tracing.Provider,
	metricsProvider metrics.Provider,
	localIdentityChecker LocalIdentityChecker,
) *Manager {
	return &Manager{
		serviceProvider:  serviceProvider,
		commLayer:        commLayer,
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

// GetManager returns an instance of *Manager, if available, an error otherwise
func GetManager(sp services.Provider) (*Manager, error) {
	s, err := sp.GetService(reflect.TypeOf((*Manager)(nil)))
	if err != nil {
		return nil, err
	}
	return s.(*Manager), nil
}

func (cm *Manager) GetService(typ reflect.Type) (interface{}, error) {
	return cm.serviceProvider.GetService(typ)
}

func (cm *Manager) RegisterFactory(id string, factory Factory) error {
	return cm.registry.RegisterFactory(id, factory)
}

func (cm *Manager) NewView(id string, in []byte) (f view.View, err error) {
	return cm.registry.NewView(id, in)
}

func (cm *Manager) RegisterResponder(responder view.View, initiatedBy interface{}) error {
	return cm.registry.RegisterResponder(responder, initiatedBy)
}

func (cm *Manager) RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy interface{}) error {
	return cm.registry.RegisterResponderWithIdentity(responder, id, initiatedBy)
}

func (cm *Manager) GetResponder(initiatedBy interface{}) (view.View, error) {
	return cm.registry.GetResponder(initiatedBy)
}

func (cm *Manager) Initiate(id string, ctx context.Context) (interface{}, error) {
	v, err := cm.registry.GetView(id)
	if err != nil {
		return nil, err
	}

	return cm.InitiateViewWithIdentity(v, cm.me(), ctx)
}

func (cm *Manager) InitiateView(view view.View, ctx context.Context) (interface{}, error) {
	return cm.InitiateViewWithIdentity(view, cm.me(), ctx)
}

func (cm *Manager) InitiateViewWithIdentity(view view.View, id view.Identity, ctx context.Context) (interface{}, error) {
	if ctx == nil {
		ctx = cm.getCurrentContext()
	}
	ctx = trace.ContextWithSpanContext(ctx, trace.SpanContextFromContext(ctx))
	viewContext, err := NewContextForInitiator(
		"",
		ctx,
		cm.serviceProvider,
		cm.commLayer,
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
	defer cm.deleteContext(id, c.ID())

	logger.DebugfContext(ctx, "[%s] InitiateView [view:%s], [ContextID:%s]", id, logging.Identifier(view), c.ID())
	res, err := c.RunView(view)
	if err != nil {
		logger.DebugfContext(ctx, "[%s] InitiateView [view:%s], [ContextID:%s] failed [%s]", id, logging.Identifier(view), c.ID(), err)
		return nil, err
	}
	logger.DebugfContext(ctx, "[%s] InitiateView [view:%s], [ContextID:%s] terminated", id, logging.Identifier(view), c.ID())
	return res, nil
}

func (cm *Manager) InitiateContext(view view.View) (view.Context, error) {
	return cm.InitiateContextFrom(cm.getCurrentContext(), view, cm.me(), "")
}

func (cm *Manager) InitiateContextWithIdentity(view view.View, id view.Identity) (view.Context, error) {
	return cm.InitiateContextFrom(cm.getCurrentContext(), view, id, "")
}

func (cm *Manager) InitiateContextWithIdentityAndID(view view.View, id view.Identity, contextID string) (view.Context, error) {
	return cm.InitiateContextFrom(cm.getCurrentContext(), view, id, contextID)
}

func (cm *Manager) InitiateContextFrom(ctx context.Context, view view.View, id view.Identity, contextID string) (view.Context, error) {
	if id.IsNone() {
		id = cm.me()
	}
	viewContext, err := NewContextForInitiator(
		contextID,
		ctx,
		cm.serviceProvider,
		cm.commLayer,
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

	logger.DebugfContext(ctx, "[%s] InitiateContext [view:%s], [ContextID:%s]\n", id, logging.Identifier(view), c.ID())

	return c, nil
}

func (cm *Manager) Start(ctx context.Context) {
	cm.setCurrentContext(ctx)

	session, err := cm.commLayer.MasterSession()
	if err != nil {
		return
	}
	for {
		ch := session.Receive()
		select {
		case msg := <-ch:
			go cm.callView(msg)
		case <-ctx.Done():
			logger.DebugfContext(ctx, "received done signal, stopping listening to messages on the master session")
			return
		}
	}
}

// Context returns a view.Context for a given contextID. If the context does not exist, an error is returned.
func (cm *Manager) Context(contextID string) (view.Context, error) {
	cm.contextsMu.RLock()
	defer cm.contextsMu.RUnlock()
	viewCtx, ok := cm.contexts[contextID]
	if !ok {
		return nil, errors.Errorf("context %s not found", contextID)
	}
	return viewCtx, nil
}

func (cm *Manager) ResolveIdentities(endpoints ...string) ([]view.Identity, error) {
	var ids []view.Identity
	for _, endpoint := range endpoints {
		id, err := cm.endpointService.GetIdentity(endpoint, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot find the idnetity at %s", endpoint)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func (cm *Manager) GetIdentifier(f view.View) string {
	return GetIdentifier(f)
}

func (cm *Manager) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
	return cm.registry.ExistResponderForCaller(caller)
}

// respond executes a given responder view
func (cm *Manager) respond(responder view.View, id view.Identity, msg *view.Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("respond triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("failed responding [%s]", r)
		}
	}()

	// get context
	viewCtx, isNew, err := cm.newContext(id, msg)
	if err != nil {
		return errors.WithMessagef(err, "failed getting context for [%s,%s,%v]", msg.ContextID, id, msg)
	}

	logger.DebugfContext(viewCtx.Context(), "[%s] Respond [from:%s], [sessionID:%s], [contextID:%s](%v), [view:%s]", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, isNew, logging.Identifier(responder))

	// if a new context has been created to run the responder,
	// then dispose the context when not needed anymore
	if isNew {
		defer cm.deleteContext(id, viewCtx.ID())
	}

	// run view
	_, err = viewCtx.RunView(responder)
	if err != nil {
		logger.DebugfContext(viewCtx.Context(), "[%s] Respond Failure [from:%s], [sessionID:%s], [contextID:%s] [%s]\n", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, err)

		// try to send error back to caller
		if err = viewCtx.Session().SendError([]byte(err.Error())); err != nil {
			logger.Error(err.Error())
		}
	}

	return nil
}

func (cm *Manager) newContext(id view.Identity, msg *view.Message) (view.Context, bool, error) {
	cm.contextsMu.Lock()
	defer cm.contextsMu.Unlock()

	// get the caller identity
	caller, err := cm.endpointService.GetIdentity(msg.FromEndpoint, msg.FromPKID)
	if err != nil {
		return nil, false, err
	}

	contextID := msg.ContextID

	// check if a viewContext already exists for the given contextID
	viewContext, ok := cm.contexts[contextID]
	if ok && viewContext.Session() != nil && viewContext.Session().Info().ID != msg.SessionID {
		// this case covers the situation where we already have an existing context between two nodes and a new session
		// is established.
		// An example for this is given in the token SDK where a node has two identities, an auditor identity and an issuer identity.

		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.DebugfContext(viewContext.Context(),
				"[%s] Found context with different session id, recreate [contextID:%s, sessionIds:%s,%s]\n",
				id,
				msg.ContextID,
				msg.SessionID,
				viewContext.Session().Info().ID,
			)
		}

		// we create a new session with the ID we received
		backend, err := cm.commLayer.NewSessionWithID(msg.SessionID, contextID, msg.FromEndpoint, msg.FromPKID, caller, msg)
		if err != nil {
			return nil, false, err
		}

		// next we need to unwrap the actual context to store the session
		vCtx, ok := viewContext.(*ChildContext)
		if !ok {
			panic("Not a child!")
		}

		vvCtx, ok := vCtx.Parent.(*Context)
		if !ok {
			panic("Not a child!")
		}
		// TODO: replace this with `vCtx.PutSession`, however, that method requires a view as input but we only have the viewID
		vvCtx.sessions.Put(msg.Caller, caller, backend)

		// we wrap our context and set our new session as the default session
		c := NewChildContextFromParentAndSession(vCtx, backend)
		cm.contexts[contextID] = c
		cm.metrics.Contexts.Set(float64(len(cm.contexts)))

		return c, false, nil
	}
	if ok {
		logger.DebugfContext(viewContext.Context(), "[%s] No new context to respond, reuse [contextID:%s]\n", id, msg.ContextID)
		return viewContext, false, nil
	}

	// next we continue with creating a new context
	logger.Debugf("[%s] Create new context to respond [contextID:%s]\n", id, msg.ContextID)
	backend, err := cm.commLayer.NewSessionWithID(msg.SessionID, contextID, msg.FromEndpoint, msg.FromPKID, caller, msg)
	if err != nil {
		return nil, false, err
	}

	ctx := trace.ContextWithSpanContext(context.Background(), trace.SpanContextFromContext(msg.Ctx))
	newCtx, err := NewContext(
		ctx,
		cm.serviceProvider,
		contextID,
		cm.commLayer,
		cm.endpointService,
		cm.identityProvider,
		id,
		backend,
		caller,
		cm.tracer,
		cm.localIdentityChecker,
	)
	if err != nil {
		return nil, false, err
	}

	c := NewChildContextFromParent(newCtx)
	cm.contexts[contextID] = c
	cm.metrics.Contexts.Set(float64(len(cm.contexts)))
	viewContext = c

	return viewContext, true, nil
}

// deleteContext removes a context from the manager and calls Dispose on the context.
func (cm *Manager) deleteContext(id view.Identity, contextID string) {
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

// callView is meant to be used to invoke a view via the p2p comm stack
func (cm *Manager) callView(msg *view.Message) {
	logger.Debugf("Will call responder view for context [%s]", msg.ContextID)
	responder, id, err := cm.ExistResponderForCaller(msg.Caller)
	if err != nil {
		// dropping message
		logger.Errorf("[%s] No responder exists for [%s]: [%s]", cm.me(), msg.String(), err)
		return
	}
	if id.IsNone() {
		id = cm.me()
	}

	if err := cm.respond(responder, id, msg); err != nil {
		logger.Errorf("[%s] error during respond [%s]", cm.me(), err)
		return
	}
}

func (cm *Manager) me() view.Identity {
	return cm.identityProvider.DefaultIdentity()
}

func (cm *Manager) getCurrentContext() context.Context {
	cm.contextsMu.Lock()
	ctx := cm.ctx
	cm.contextsMu.Unlock()
	return ctx
}

func (cm *Manager) setCurrentContext(ctx context.Context) {
	cm.contextsMu.Lock()
	cm.ctx = ctx
	cm.contextsMu.Unlock()
}
