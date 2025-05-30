/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager

import (
	"context"
	"reflect"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/registry"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
)

const (
	SuccessLabel       tracing.LabelName = "success"
	ViewLabel          tracing.LabelName = "view"
	InitiatorViewLabel tracing.LabelName = "initiator_view"
)

var logger = logging.MustGetLogger()

type manager struct {
	sp driver.ServiceProvider

	commLayer        CommLayer
	endpointService  driver.EndpointService
	identityProvider driver.IdentityProvider

	ctx context.Context

	contextsSync sync.RWMutex

	contexts map[string]disposableContext

	viewProvider *registry.ViewProvider

	viewTracer trace.Tracer
	m          *Metrics
}

func New(serviceProvider driver.ServiceProvider, commLayer CommLayer, endpointService driver.EndpointService, identityProvider driver.IdentityProvider, viewProvider *registry.ViewProvider, provider trace.TracerProvider, metricsProvider metrics.Provider) *manager {
	return &manager{
		sp:               serviceProvider,
		commLayer:        commLayer,
		endpointService:  endpointService,
		identityProvider: identityProvider,

		contexts:     map[string]disposableContext{},
		viewProvider: viewProvider,

		viewTracer: provider.Tracer("view", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "fsc",
			LabelNames: []string{SuccessLabel, ViewLabel, InitiatorViewLabel},
		})),
		m: newMetrics(metricsProvider),
	}
}

func (cm *manager) GetService(typ reflect.Type) (interface{}, error) {
	return cm.sp.GetService(typ)
}

func (cm *manager) RegisterFactory(id string, factory driver.Factory) error {
	return cm.viewProvider.RegisterFactory(id, factory)
}

func (cm *manager) NewView(id string, in []byte) (f view.View, err error) {
	return cm.viewProvider.NewView(id, in)
}

func (cm *manager) RegisterResponder(responder view.View, initiatedBy interface{}) error {
	return cm.viewProvider.RegisterResponder(responder, initiatedBy)
}

func (cm *manager) RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy interface{}) error {
	return cm.viewProvider.RegisterResponderWithIdentity(responder, id, initiatedBy)
}

func (cm *manager) GetResponder(initiatedBy interface{}) (view.View, error) {
	return cm.viewProvider.GetResponder(initiatedBy)
}

func (cm *manager) Initiate(id string, ctx context.Context) (interface{}, error) {
	v, err := cm.viewProvider.GetView(id)
	if err != nil {
		return nil, err
	}

	return cm.InitiateViewWithIdentity(v, cm.me(), ctx)
}

func (cm *manager) InitiateView(view view.View, ctx context.Context) (interface{}, error) {
	return cm.InitiateViewWithIdentity(view, cm.me(), ctx)
}

func (cm *manager) InitiateViewWithIdentity(view view.View, id view.Identity, c context.Context) (interface{}, error) {
	// Create the context
	cm.contextsSync.Lock()
	ctx := cm.ctx
	cm.contextsSync.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = trace.ContextWithSpanContext(ctx, trace.SpanContextFromContext(c))

	viewContext, err := NewContextForInitiator("", ctx, cm.sp, cm.commLayer, cm.endpointService, cm.identityProvider, id, view, cm.viewTracer)
	if err != nil {
		return nil, err
	}
	childContext := &childContext{ParentContext: viewContext}
	cm.contextsSync.Lock()
	cm.contexts[childContext.ID()] = childContext
	cm.m.Contexts.Set(float64(len(cm.contexts)))
	cm.contextsSync.Unlock()
	defer cm.deleteContext(id, childContext.ID())

	logger.Debugf("[%s] InitiateView [view:%s], [ContextID:%s]", id, logging.Identifier(view), childContext.ID())
	res, err := childContext.RunView(view)
	if err != nil {
		logger.Debugf("[%s] InitiateView [view:%s], [ContextID:%s] failed [%s]", id, logging.Identifier(view), childContext.ID(), err)
		return nil, err
	}
	logger.Debugf("[%s] InitiateView [view:%s], [ContextID:%s] terminated", id, logging.Identifier(view), childContext.ID())
	return res, nil
}

func (cm *manager) InitiateContext(view view.View) (view.Context, error) {
	return cm.InitiateContextFrom(cm.ctx, view, cm.me(), "")
}

func (cm *manager) InitiateContextWithIdentity(view view.View, id view.Identity) (view.Context, error) {
	return cm.InitiateContextFrom(cm.ctx, view, id, "")
}

func (cm *manager) InitiateContextWithIdentityAndID(view view.View, id view.Identity, contextID string) (view.Context, error) {
	return cm.InitiateContextFrom(cm.ctx, view, id, contextID)
}

func (cm *manager) InitiateContextFrom(ctx context.Context, view view.View, id view.Identity, contextID string) (view.Context, error) {
	if id.IsNone() {
		id = cm.me()
	}
	viewContext, err := NewContextForInitiator(contextID, ctx, cm.sp, cm.commLayer, cm.endpointService, cm.identityProvider, id, view, cm.viewTracer)
	if err != nil {
		return nil, err
	}
	childContext := &childContext{ParentContext: viewContext}
	cm.contextsSync.Lock()
	cm.contexts[childContext.ID()] = childContext
	cm.m.Contexts.Set(float64(len(cm.contexts)))
	cm.contextsSync.Unlock()

	logger.Debugf("[%s] InitiateContext [view:%s], [ContextID:%s]\n", id, logging.Identifier(view), childContext.ID())

	return childContext, nil
}

func (cm *manager) Start(ctx context.Context) {
	cm.ctx = ctx
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
			logger.Debugf("received done signal, stopping listening to messages on the master session")
			return
		}
	}
}

func (cm *manager) Context(contextID string) (view.Context, error) {
	cm.contextsSync.RLock()
	defer cm.contextsSync.RUnlock()
	context, ok := cm.contexts[contextID]
	if !ok {
		return nil, errors.Errorf("context %s not found", contextID)
	}
	return context, nil
}

func (cm *manager) ResolveIdentities(endpoints ...string) ([]view.Identity, error) {
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

func (cm *manager) GetIdentifier(f view.View) string {
	return registry.GetIdentifier(f)
}

func (cm *manager) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
	return cm.viewProvider.ExistResponderForCaller(caller)
}

func (cm *manager) respond(responder view.View, id view.Identity, msg *view.Message) (ctx view.Context, res interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("respond triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("failed responding [%s]", r)
		}
	}()

	// get context
	var isNew bool
	ctx, isNew, err = cm.newContext(id, msg)
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "failed getting context for [%s,%s,%v]", msg.ContextID, id, msg)
	}

	logger.Debugf("[%s] Respond [from:%s], [sessionID:%s], [contextID:%s](%v), [view:%s]", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, isNew, logging.Identifier(responder))

	// todo: if a new context has been created to run the responder,
	// then dispose the context when the responder terminates
	// run view
	if isNew {
		// delete context at the end of the execution
		res, err = func(ctx view.Context, responder view.View) (interface{}, error) {
			defer func() {
				// TODO: this is a workaround
				// give some time to flush anything can be in queues
				time.Sleep(5 * time.Second)
				cm.deleteContext(id, ctx.ID())
			}()
			return ctx.RunView(responder)
		}(ctx, responder)
	} else {
		res, err = ctx.RunView(responder)
	}
	if err != nil {
		logger.Debugf("[%s] Respond Failure [from:%s], [sessionID:%s], [contextID:%s] [%s]\n", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, err)
	}
	return ctx, res, err
}

func (cm *manager) newContext(id view.Identity, msg *view.Message) (view.Context, bool, error) {
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
			logger.Debugf(
				"[%s] Found context with different session id, recreate [contextID:%s, sessionIds:%s,%s]\n",
				id,
				msg.ContextID,
				msg.SessionID,
				viewContext.Session().Info().ID,
			)
		}
		viewContext.Dispose()
		delete(cm.contexts, contextID)
		cm.m.Contexts.Set(float64(len(cm.contexts)))
		ok = false
	}
	if ok {
		logger.Debugf("[%s] No new context to respond, reuse [contextID:%s]\n", id, msg.ContextID)
		return viewContext, false, nil
	}

	logger.Debugf("[%s] Create new context to respond [contextID:%s]\n", id, msg.ContextID)
	backend, err := cm.commLayer.NewSessionWithID(msg.SessionID, contextID, msg.FromEndpoint, msg.FromPKID, caller, msg)
	if err != nil {
		return nil, false, err
	}
	ctx := trace.ContextWithSpanContext(cm.ctx, trace.SpanContextFromContext(msg.Ctx))
	newCtx, err := NewContext(ctx, cm.sp, contextID, cm.commLayer, cm.endpointService, cm.identityProvider, id, backend, caller, cm.viewTracer)
	if err != nil {
		return nil, false, err
	}
	childContext := &childContext{ParentContext: newCtx}
	cm.contexts[contextID] = childContext
	cm.m.Contexts.Set(float64(len(cm.contexts)))
	viewContext = childContext

	return viewContext, true, nil
}

func (cm *manager) deleteContext(id view.Identity, contextID string) {
	cm.contextsSync.Lock()
	defer cm.contextsSync.Unlock()

	logger.Debugf("[%s] Delete context [contextID:%s]\n", id, contextID)
	// dispose context
	if context, ok := cm.contexts[contextID]; ok {
		context.Dispose()
		delete(cm.contexts, contextID)
		cm.m.Contexts.Set(float64(len(cm.contexts)))
	}
}

func (cm *manager) existResponder(msg *view.Message) (view.View, view.Identity, error) {
	return cm.ExistResponderForCaller(msg.Caller)
}

func (cm *manager) callView(msg *view.Message) {
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

	ctx, _, err := cm.respond(responder, id, msg)
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
			logger.Errorf(err.Error())
		}
	}
}

func (cm *manager) me() view.Identity {
	return cm.identityProvider.DefaultIdentity()
}
