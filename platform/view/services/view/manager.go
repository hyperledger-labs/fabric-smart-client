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
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
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

type Manager struct {
	sp services.Provider

	commLayer        CommLayer
	endpointService  EndpointService
	identityProvider IdentityProvider

	ctx context.Context

	contextsSync sync.RWMutex

	contexts map[string]disposableContext

	registry *Registry

	viewTracer           trace.Tracer
	m                    *Metrics
	localIdentityChecker LocalIdentityChecker
}

func NewManager(
	serviceProvider services.Provider,
	commLayer CommLayer,
	endpointService EndpointService,
	identityProvider IdentityProvider,
	viewProvider *Registry,
	provider trace.TracerProvider,
	metricsProvider metrics.Provider,
	localIdentityChecker LocalIdentityChecker,
) *Manager {
	return &Manager{
		sp:               serviceProvider,
		commLayer:        commLayer,
		endpointService:  endpointService,
		identityProvider: identityProvider,

		contexts: map[string]disposableContext{},
		registry: viewProvider,

		viewTracer: provider.Tracer("view", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "fsc",
			LabelNames: []string{SuccessLabel, ViewLabel, InitiatorViewLabel},
		})),
		m:                    newMetrics(metricsProvider),
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

func (m *Manager) GetService(typ reflect.Type) (interface{}, error) {
	return m.sp.GetService(typ)
}

func (m *Manager) RegisterFactory(id string, factory Factory) error {
	return m.registry.RegisterFactory(id, factory)
}

func (m *Manager) NewView(id string, in []byte) (f view.View, err error) {
	return m.registry.NewView(id, in)
}

func (m *Manager) RegisterResponder(responder view.View, initiatedBy interface{}) error {
	return m.registry.RegisterResponder(responder, initiatedBy)
}

func (m *Manager) RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy interface{}) error {
	return m.registry.RegisterResponderWithIdentity(responder, id, initiatedBy)
}

func (m *Manager) GetResponder(initiatedBy interface{}) (view.View, error) {
	return m.registry.GetResponder(initiatedBy)
}

func (m *Manager) Initiate(ctx context.Context, id string) (interface{}, error) {
	v, err := m.registry.GetView(id)
	if err != nil {
		return nil, err
	}

	return m.InitiateViewWithIdentity(ctx, v, m.me())
}

func (m *Manager) InitiateView(ctx context.Context, view view.View) (interface{}, error) {
	return m.InitiateViewWithIdentity(ctx, view, m.me())
}

func (m *Manager) InitiateViewWithIdentity(c context.Context, view view.View, id view.Identity) (interface{}, error) {
	// Create the context
	m.contextsSync.Lock()
	ctx := m.ctx
	m.contextsSync.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = trace.ContextWithSpanContext(ctx, trace.SpanContextFromContext(c))

	viewContext, err := NewContextForInitiator(
		"",
		ctx,
		m.sp,
		m.commLayer,
		m.endpointService,
		m.identityProvider,
		id,
		view,
		m.viewTracer,
		m.localIdentityChecker,
	)
	if err != nil {
		return nil, err
	}
	childContext := &childContext{ParentContext: viewContext}
	m.contextsSync.Lock()
	m.contexts[childContext.ID()] = childContext
	m.m.Contexts.Set(float64(len(m.contexts)))
	m.contextsSync.Unlock()
	defer m.deleteContext(id, childContext.ID())

	logger.Debugf("[%s] InitiateView [view:%s], [ContextID:%s]", id, logging.Identifier(view), childContext.ID())
	res, err := childContext.RunView(view)
	if err != nil {
		logger.Debugf("[%s] InitiateView [view:%s], [ContextID:%s] failed [%s]", id, logging.Identifier(view), childContext.ID(), err)
		return nil, err
	}
	logger.Debugf("[%s] InitiateView [view:%s], [ContextID:%s] terminated", id, logging.Identifier(view), childContext.ID())
	return res, nil
}

func (m *Manager) InitiateContext(view view.View) (view.Context, error) {
	return m.InitiateContextFrom(m.ctx, view, m.me(), "")
}

func (m *Manager) InitiateContextWithIdentity(view view.View, id view.Identity) (view.Context, error) {
	return m.InitiateContextFrom(m.ctx, view, id, "")
}

func (m *Manager) InitiateContextWithIdentityAndID(view view.View, id view.Identity, contextID string) (view.Context, error) {
	return m.InitiateContextFrom(m.ctx, view, id, contextID)
}

func (m *Manager) InitiateContextFrom(ctx context.Context, view view.View, id view.Identity, contextID string) (view.Context, error) {
	if id.IsNone() {
		id = m.me()
	}
	viewContext, err := NewContextForInitiator(
		contextID,
		ctx,
		m.sp,
		m.commLayer,
		m.endpointService,
		m.identityProvider,
		id,
		view,
		m.viewTracer,
		m.localIdentityChecker,
	)
	if err != nil {
		return nil, err
	}
	childContext := &childContext{ParentContext: viewContext}
	m.contextsSync.Lock()
	m.contexts[childContext.ID()] = childContext
	m.m.Contexts.Set(float64(len(m.contexts)))
	m.contextsSync.Unlock()

	logger.Debugf("[%s] InitiateContext [view:%s], [ContextID:%s]\n", id, logging.Identifier(view), childContext.ID())

	return childContext, nil
}

func (m *Manager) Start(ctx context.Context) {
	m.ctx = ctx
	session, err := m.commLayer.MasterSession()
	if err != nil {
		return
	}
	for {
		ch := session.Receive()
		select {
		case msg := <-ch:
			go m.callView(msg)
		case <-ctx.Done():
			logger.Debugf("received done signal, stopping listening to messages on the master session")
			return
		}
	}
}

func (m *Manager) Context(contextID string) (view.Context, error) {
	m.contextsSync.RLock()
	defer m.contextsSync.RUnlock()
	context, ok := m.contexts[contextID]
	if !ok {
		return nil, errors.Errorf("context %s not found", contextID)
	}
	return context, nil
}

func (m *Manager) ResolveIdentities(endpoints ...string) ([]view.Identity, error) {
	var ids []view.Identity
	for _, endpoint := range endpoints {
		id, err := m.endpointService.GetIdentity(endpoint, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot find the idnetity at %s", endpoint)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func (m *Manager) GetIdentifier(f view.View) string {
	return GetIdentifier(f)
}

func (m *Manager) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
	return m.registry.ExistResponderForCaller(caller)
}

func (m *Manager) respond(responder view.View, id view.Identity, msg *view.Message) (ctx view.Context, res interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("respond triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("failed responding [%s]", r)
		}
	}()

	// get context
	var isNew bool
	ctx, isNew, err = m.newContext(id, msg)
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
				m.deleteContext(id, ctx.ID())
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

func (m *Manager) newContext(id view.Identity, msg *view.Message) (view.Context, bool, error) {
	m.contextsSync.Lock()
	defer m.contextsSync.Unlock()

	caller, err := m.endpointService.GetIdentity(msg.FromEndpoint, msg.FromPKID)
	if err != nil {
		return nil, false, err
	}

	contextID := msg.ContextID
	viewContext, ok := m.contexts[contextID]
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
		delete(m.contexts, contextID)
		m.m.Contexts.Set(float64(len(m.contexts)))
		ok = false
	}
	if ok {
		logger.Debugf("[%s] No new context to respond, reuse [contextID:%s]\n", id, msg.ContextID)
		return viewContext, false, nil
	}

	logger.Debugf("[%s] Create new context to respond [contextID:%s]\n", id, msg.ContextID)
	backend, err := m.commLayer.NewSessionWithID(msg.SessionID, contextID, msg.FromEndpoint, msg.FromPKID, caller, msg)
	if err != nil {
		return nil, false, err
	}
	ctx := trace.ContextWithSpanContext(m.ctx, trace.SpanContextFromContext(msg.Ctx))
	newCtx, err := NewContext(
		ctx,
		m.sp,
		contextID,
		m.commLayer,
		m.endpointService,
		m.identityProvider,
		id,
		backend,
		caller,
		m.viewTracer,
		m.localIdentityChecker,
	)
	if err != nil {
		return nil, false, err
	}
	childContext := &childContext{ParentContext: newCtx}
	m.contexts[contextID] = childContext
	m.m.Contexts.Set(float64(len(m.contexts)))
	viewContext = childContext

	return viewContext, true, nil
}

func (m *Manager) deleteContext(id view.Identity, contextID string) {
	m.contextsSync.Lock()
	defer m.contextsSync.Unlock()

	logger.Debugf("[%s] Delete context [contextID:%s]\n", id, contextID)
	// dispose context
	if context, ok := m.contexts[contextID]; ok {
		context.Dispose()
		delete(m.contexts, contextID)
		m.m.Contexts.Set(float64(len(m.contexts)))
	}
}

func (m *Manager) existResponder(msg *view.Message) (view.View, view.Identity, error) {
	return m.ExistResponderForCaller(msg.Caller)
}

func (m *Manager) callView(msg *view.Message) {
	logger.Debugf("Will call responder view for context [%s]", msg.ContextID)
	responder, id, err := m.existResponder(msg)
	if err != nil {
		// TODO: No responder exists for this message
		// Let's cache it for a while an re-post
		logger.Errorf("[%s] No responder exists for [%s]: [%s]", m.me(), msg.String(), err)
		return
	}
	if id.IsNone() {
		id = m.me()
	}

	ctx, _, err := m.respond(responder, id, msg)
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

func (m *Manager) me() view.Identity {
	return m.identityProvider.DefaultIdentity()
}
