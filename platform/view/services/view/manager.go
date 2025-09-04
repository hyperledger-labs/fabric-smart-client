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

type Manager struct {
	serviceProvider services.Provider

	commLayer            CommLayer
	endpointService      EndpointService
	identityProvider     IdentityProvider
	registry             *Registry
	tracer               trace.Tracer
	metrics              *Metrics
	localIdentityChecker LocalIdentityChecker

	contextsSync sync.RWMutex
	ctx          context.Context
	contexts     map[string]disposableContext
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

		contexts: map[string]disposableContext{},
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

func (cm *Manager) InitiateViewWithIdentity(view view.View, id view.Identity, c context.Context) (interface{}, error) {
	// Create the context
	cm.contextsSync.Lock()
	ctx := cm.ctx
	cm.contextsSync.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = trace.ContextWithSpanContext(ctx, trace.SpanContextFromContext(c))

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
	childContext := &childContext{ParentContext: viewContext}
	cm.contextsSync.Lock()
	cm.contexts[childContext.ID()] = childContext
	cm.metrics.Contexts.Set(float64(len(cm.contexts)))
	cm.contextsSync.Unlock()
	defer cm.deleteContext(id, childContext.ID())

	logger.DebugfContext(c, "[%s] InitiateView [view:%s], [ContextID:%s]", id, logging.Identifier(view), childContext.ID())
	res, err := childContext.RunView(view)
	if err != nil {
		logger.DebugfContext(c, "[%s] InitiateView [view:%s], [ContextID:%s] failed [%s]", id, logging.Identifier(view), childContext.ID(), err)
		return nil, err
	}
	logger.DebugfContext(c, "[%s] InitiateView [view:%s], [ContextID:%s] terminated", id, logging.Identifier(view), childContext.ID())
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
	childContext := &childContext{ParentContext: viewContext}
	cm.contextsSync.Lock()
	cm.contexts[childContext.ID()] = childContext
	cm.metrics.Contexts.Set(float64(len(cm.contexts)))
	cm.contextsSync.Unlock()

	logger.DebugfContext(ctx, "[%s] InitiateContext [view:%s], [ContextID:%s]\n", id, logging.Identifier(view), childContext.ID())

	return childContext, nil
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

func (cm *Manager) Context(contextID string) (view.Context, error) {
	cm.contextsSync.RLock()
	defer cm.contextsSync.RUnlock()
	context, ok := cm.contexts[contextID]
	if !ok {
		return nil, errors.Errorf("context %s not found", contextID)
	}
	return context, nil
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
// the caller is responsible to use the cleanup method to free any resources
func (cm *Manager) respond(responder view.View, id view.Identity, msg *view.Message) (ctx view.Context, res interface{}, cleanup func(), err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("respond triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("failed responding [%s]", r)
		}
	}()

	cleanup = func() {}

	// get context
	var isNew bool
	ctx, isNew, err = cm.newContext(id, msg)
	if err != nil {
		return nil, nil, cleanup, errors.WithMessagef(err, "failed getting context for [%s,%s,%v]", msg.ContextID, id, msg)
	}

	logger.DebugfContext(ctx.Context(), "[%s] Respond [from:%s], [sessionID:%s], [contextID:%s](%v), [view:%s]", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, isNew, logging.Identifier(responder))

	// if a new context has been created to run the responder,
	// then dispose the context when not needed anymore
	if isNew {
		cleanup = func() {
			cm.deleteContext(id, ctx.ID())
		}
	}

	// run view
	res, err = ctx.RunView(responder)
	if err != nil {
		logger.DebugfContext(ctx.Context(), "[%s] Respond Failure [from:%s], [sessionID:%s], [contextID:%s] [%s]\n", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, err)
	}

	return ctx, res, cleanup, err
}

func (cm *Manager) newContext(id view.Identity, msg *view.Message) (view.Context, bool, error) {
	cm.contextsSync.Lock()
	defer cm.contextsSync.Unlock()

	caller, err := cm.endpointService.GetIdentity(msg.FromEndpoint, msg.FromPKID)
	if err != nil {
		return nil, false, err
	}

	contextID := msg.ContextID
	viewContext, ok := cm.contexts[contextID]
	if ok && viewContext.Session() != nil && viewContext.Session().Info().ID != msg.SessionID {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.DebugfContext(viewContext.Context(),
				"[%s] Found context with different session id, recreate [contextID:%s, sessionIds:%s,%s]\n",
				id,
				msg.ContextID,
				msg.SessionID,
				viewContext.Session().Info().ID,
			)
		}
		viewContext.Dispose()
		delete(cm.contexts, contextID)
		cm.metrics.Contexts.Set(float64(len(cm.contexts)))
		ok = false
	}
	if ok {
		logger.DebugfContext(viewContext.Context(), "[%s] No new context to respond, reuse [contextID:%s]\n", id, msg.ContextID)
		return viewContext, false, nil
	}

	logger.Debugf("[%s] Create new context to respond [contextID:%s]\n", id, msg.ContextID)
	backend, err := cm.commLayer.NewSessionWithID(msg.SessionID, contextID, msg.FromEndpoint, msg.FromPKID, caller, msg)
	if err != nil {
		return nil, false, err
	}
	ctx := trace.ContextWithSpanContext(cm.ctx, trace.SpanContextFromContext(msg.Ctx))
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
	childContext := &childContext{ParentContext: newCtx}
	cm.contexts[contextID] = childContext
	cm.metrics.Contexts.Set(float64(len(cm.contexts)))
	viewContext = childContext

	return viewContext, true, nil
}

func (cm *Manager) deleteContext(id view.Identity, contextID string) {
	cm.contextsSync.Lock()
	defer cm.contextsSync.Unlock()

	logger.Debugf("[%s] Delete context [contextID:%s]\n", id, contextID)
	// dispose context
	if context, ok := cm.contexts[contextID]; ok {
		context.Dispose()
		delete(cm.contexts, contextID)
		cm.metrics.Contexts.Set(float64(len(cm.contexts)))
	}
}

func (cm *Manager) existResponder(msg *view.Message) (view.View, view.Identity, error) {
	return cm.ExistResponderForCaller(msg.Caller)
}

func (cm *Manager) callView(msg *view.Message) {
	logger.Debugf("Will call responder view for context [%s]", msg.ContextID)
	responder, id, err := cm.existResponder(msg)
	if err != nil {
		// TODO: No responder exists for this message
		// Let's cache it for a while an re-post
		logger.Errorf("[%s] No responder exists for [%s]: [%s]", cm.me(), msg.String(), err)
		return
	}
	if id.IsNone() {
		id = cm.me()
	}

	ctx, _, cleanup, err := cm.respond(responder, id, msg)
	defer cleanup()
	if err != nil {
		logger.Errorf("failed responding [%v, %v], err: [%s]", logging.Identifier(responder), msg.String(), err)
		if ctx == nil {
			logger.Debugf("no context set, returning")
			return
		}

		// Return the error to the caller
		logger.Debugf("return the error to the caller [%s]", err)
		err = ctx.Session().SendError([]byte(err.Error()))
		if err != nil {
			logger.Error(err.Error())
		}
	}
}

func (cm *Manager) me() view.Identity {
	return cm.identityProvider.DefaultIdentity()
}

func (cm *Manager) getCurrentContext() context.Context {
	cm.contextsSync.Lock()
	ctx := cm.ctx
	cm.contextsSync.Unlock()
	return ctx
}

func (cm *Manager) setCurrentContext(ctx context.Context) {
	cm.contextsSync.Lock()
	cm.ctx = ctx
	cm.contextsSync.Unlock()
}
