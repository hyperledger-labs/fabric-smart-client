/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"bytes"
	"context"
	"reflect"
	"runtime/debug"
	"sync"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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
	runner           Runner

	contexts map[string]DisposableContext
	// responders holds the responder contexts created by NewResponderContext, by context ID.
	responders map[string]*responderContext
	contextsMu sync.RWMutex
}

// responderContext is a responder context shared by the responders that run in it. It is
// disposed, and its go context cancelled, when the last of them releases it. Its fields are
// guarded by Manager.contextsMu.
type responderContext struct {
	parent     ParentContext
	cancel     context.CancelFunc
	responders int
}

// NewManager returns a new instance of the view manager.
func NewManager(
	identityProvider IdentityProvider,
	registry *Registry,
	metrics *Metrics,
	contextFactory ContextFactory,
	runner Runner,
) *Manager {
	return &Manager{
		identityProvider: identityProvider,

		contexts:   map[string]DisposableContext{},
		responders: map[string]*responderContext{},
		registry:   registry,

		metrics:        metrics,
		contextFactory: contextFactory,

		runner: runner,
	}
}

// GetManager returns an instance of *Manager, if available, an error otherwise.
func GetManager(sp services.Provider) (*Manager, error) {
	s, err := sp.GetService(reflect.TypeFor[*Manager]())
	if err != nil {
		return nil, err
	}
	m, ok := s.(*Manager)
	if !ok {
		return nil, errors.Errorf("unexpected service type [%T] for view manager", s)
	}
	return m, nil
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
func (cm *Manager) RegisterResponder(responder view.View, initiatedBy any) error {
	return cm.registry.RegisterResponder(responder, initiatedBy)
}

// RegisterResponderWithIdentity registers a responder view for the given initiator view and responder identity.
func (cm *Manager) RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy any) error {
	return cm.registry.RegisterResponderWithIdentity(responder, id, initiatedBy)
}

// GetResponder returns the responder view for the given initiator view.
func (cm *Manager) GetResponder(initiatedBy any) (view.View, error) {
	return cm.registry.GetResponder(initiatedBy)
}

// Initiate initiates a protocol for the given view ID.
func (cm *Manager) Initiate(ctx context.Context, id string) (any, error) {
	v, err := cm.registry.GetView(id)
	if err != nil {
		return nil, err
	}

	return cm.InitiateViewWithIdentity(ctx, v, cm.identityProvider.DefaultIdentity())
}

// InitiateView initiates a protocol for the given view.
func (cm *Manager) InitiateView(ctx context.Context, view view.View) (any, error) {
	return cm.InitiateViewWithIdentity(ctx, view, cm.identityProvider.DefaultIdentity())
}

// InitiateViewWithIdentity initiates a protocol for the given view and
// initiator identity. It returns an error if ctx is nil.
func (cm *Manager) InitiateViewWithIdentity(ctx context.Context, view view.View, id view.Identity) (any, error) {
	if ctx == nil {
		return nil, errors.New("context is nil")
	}
	ctx = trace.ContextWithSpanContext(ctx, trace.SpanContextFromContext(ctx))
	c, err := cm.newChildContextForInitiator(ctx, view, id, "")
	if err != nil {
		return nil, err
	}
	defer cm.DeleteContext(c.ID())

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.DebugfContext(ctx, "[%s] InitiateView [view:%s], [ContextID:%s], from [%s]", id, logging.Identifier(view), c.ID(), string(debug.Stack()))
	}
	res, err := cm.runner.RunView(c, view)
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
// It is responsibility of the caller to delete context from this manager when not needed anymore.
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
// It is responsibility of the caller to remove context from this manager when not needed anymore.
func (cm *Manager) RegisterContext(contextID string, ctx DisposableContext) error {
	cm.contextsMu.Lock()
	defer cm.contextsMu.Unlock()
	cm.contexts[contextID] = ctx
	cm.metrics.Contexts.Set(float64(len(cm.contexts)))

	return nil
}

// NewResponderContext returns a context to be used to respond to an incoming message on the
// given session, and the func that releases it. The caller must call release when its responder
// returns; release also runs when ctx is done, and extra calls are no-ops.
//
// contextID and the session ID are chosen by the remote peer; a session is identified by its ID
// and remote PKID. If no context with that ID exists, a new responder context is created. If a
// responder context with that ID exists and its current session (of the responder that created
// it, or that attached last) is another session, session attaches: it is registered in the
// context, so that Dispose deletes it, and a child context with session as its default session
// replaces the context in the manager. Any participant that knows the context ID can attach.
//
// Several responders can run in one context. Each one sees ctx as its go context, so that it is
// not cancelled when another responder returns; the context registered in the manager uses the
// shared go context instead. The context is removed from the manager and disposed, deleting
// every session registered in it, when the last responder releases it.
//
// It returns ErrInvalidSessionID for an empty session ID, and ErrNotResponderContext if the
// context with that ID was not created by NewResponderContext (for example, an initiator
// context), so that an incoming session cannot join a protocol this node initiated.
//
// On error, no context holds session and the caller must delete it.
func (cm *Manager) NewResponderContext(ctx context.Context, contextID string, session view.Session, me, remote view.Identity) (view.Context, func(), error) {
	if ctx == nil {
		return nil, nil, errors.New("context is nil")
	}
	cm.contextsMu.Lock()
	defer cm.contextsMu.Unlock()

	if me.IsNone() {
		me = cm.identityProvider.DefaultIdentity()
	}

	info := session.Info()
	if len(info.ID) == 0 {
		return nil, nil, errors.Wrapf(ErrInvalidSessionID, "incoming session for context [%s]", contextID)
	}

	if viewContext, ok := cm.contexts[contextID]; ok {
		rc, ok := cm.responders[contextID]
		if !ok {
			return nil, nil, errors.Wrapf(ErrNotResponderContext, "context [%s]", contextID)
		}
		current := viewContext.Session()
		// A session is identified by its ID and remote PKID: the same ID from another party
		// is a different session and must be registered, so that the context replies on it
		// and Dispose deletes it.
		attach := !sameSession(current.Info(), info)
		if attach {
			// The key matches sameSession, so distinct sessions never share an entry. The default
			// session and the sessions of views are keyed by identity, not PKID, so a peer-chosen
			// session ID cannot select their entries.
			if err := rc.parent.PutSessionByID(info.ID, info.RemotePKID, session); err != nil {
				return nil, nil, errors.Wrapf(err, "failed registering session for [%s]", info.Caller)
			}
			current = session
		}
		logger.DebugfContext(ctx, "[%s] Join context to respond [contextID:%s][attach:%v]", me, contextID, attach)
		if attach {
			cm.contexts[contextID] = NewChildContextFromParentAndSession(rc.parent, current)
		}
		c := NewChildContextFromParentAndSession(rc.parent, current)
		c.ctx = ctx
		return c, cm.acquire(ctx, contextID, rc), nil
	}

	logger.DebugfContext(ctx, "[%s] Create new context to respond [contextID:%s]", me, contextID)
	// The shared context outlives the responder that creates it, so its go context keeps the
	// values of ctx but is cancelled only when the last responder releases it.
	parentCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	newCtx, err := cm.contextFactory.NewForResponder(
		parentCtx,
		contextID,
		me,
		session,
		remote,
	)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	rc := &responderContext{parent: newCtx, cancel: cancel}
	cm.responders[contextID] = rc
	cm.contexts[contextID] = NewChildContextFromParent(newCtx)
	cm.metrics.Contexts.Set(float64(len(cm.contexts)))
	c := NewChildContextFromParent(newCtx)
	c.ctx = ctx

	return c, cm.acquire(ctx, contextID, rc), nil
}

// acquire adds a responder to rc and returns the func that releases it, once, when called or
// when ctx is done. The caller must hold cm.contextsMu.
func (cm *Manager) acquire(ctx context.Context, contextID string, rc *responderContext) func() {
	rc.responders++
	release := sync.OnceFunc(func() { cm.release(contextID, rc) })
	stop := context.AfterFunc(ctx, release)
	return func() {
		stop()
		release()
	}
}

// release removes a responder from rc, and disposes rc when it was the last one. rc is disposed
// and removed from the manager only if it is still registered under contextID: DeleteContext
// has already disposed a context it removed, and a newer context may reuse the ID.
func (cm *Manager) release(contextID string, rc *responderContext) {
	cm.contextsMu.Lock()
	defer cm.contextsMu.Unlock()

	rc.responders--
	if rc.responders > 0 {
		return
	}
	defer rc.cancel()
	if cm.responders[contextID] == rc {
		logger.Debugf("last responder returned, dispose context [%s]", contextID)
		rc.parent.Dispose()
		delete(cm.responders, contextID)
		delete(cm.contexts, contextID)
		cm.metrics.Contexts.Set(float64(len(cm.contexts)))
	}
}

// GetIdentifier returns the identifier for the given view.
func (*Manager) GetIdentifier(f view.View) string {
	return GetIdentifier(f)
}

// ExistResponderForCaller returns the responder view and identity for the given caller identifier.
func (cm *Manager) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
	return cm.registry.ExistResponderForCaller(caller)
}

// DeleteContext removes a context from the manager and calls Dispose on the context. A responder
// context is disposed immediately, including the sessions of responders still running in it;
// responders release their context with the func returned by NewResponderContext instead.
func (cm *Manager) DeleteContext(contextID string) {
	cm.contextsMu.Lock()
	defer cm.contextsMu.Unlock()

	delete(cm.responders, contextID)

	logger.Debugf("deleting context [%s]", contextID)
	// dispose context
	if viewCtx, ok := cm.contexts[contextID]; ok {
		logger.Debugf("found, dispose context [%s]", contextID)
		viewCtx.Dispose()
		delete(cm.contexts, contextID)
		cm.metrics.Contexts.Set(float64(len(cm.contexts)))
	}
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

func sameSession(a, b view.SessionInfo) bool {
	return a.ID == b.ID && bytes.Equal(a.RemotePKID, b.RemotePKID)
}
