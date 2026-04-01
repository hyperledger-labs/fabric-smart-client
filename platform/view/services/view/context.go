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

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.opentelemetry.io/otel/trace"
)

// LocalIdentityChecker models the dependency to the view-sdk's sig service.
// It allows checking if a given identity is local to the node.
//
//go:generate counterfeiter -o mock/local_identity_checker.go -fake-name LocalIdentityChecker . LocalIdentityChecker
type LocalIdentityChecker interface {
	// IsMe returns true if the passed identity is local to the node.
	IsMe(ctx context.Context, id view.Identity) bool
}

// EndpointService models the dependency to the view-sdk's endpoint service.
// It provides methods to retrieve identities and resolvers for endpoints.
//
//go:generate counterfeiter -o mock/resolver.go -fake-name EndpointService . EndpointService
type EndpointService interface {
	// GetIdentity returns the identity for the given endpoint and public key ID.
	GetIdentity(endpoint string, pkID []byte) (view.Identity, error)
	// Resolver returns the resolver for the given party.
	Resolver(ctx context.Context, party view.Identity) (*endpoint.Resolver, []byte, error)
}

// IdentityProvider models the dependency to the view-sdk's identity provider.
// It provides methods to retrieve identities by label and the default identity.
//
//go:generate counterfeiter -o mock/identity_provider.go -fake-name IdentityProvider . IdentityProvider
type IdentityProvider interface {
	// Identity returns the identity for the given label.
	Identity(string) view.Identity
	// DefaultIdentity returns the default identity.
	DefaultIdentity() view.Identity
}

// contextFactory is an implementation of the ContextFactory interface.
type contextFactory struct {
	serviceProvider      services.Provider
	sessionFactory       SessionFactory
	endpointService      EndpointService
	identityProvider     IdentityProvider
	registry             *Registry
	tracer               trace.Tracer
	metrics              *Metrics
	localIdentityChecker LocalIdentityChecker
}

// NewContextFactory returns a new instance of the context factory.
func NewContextFactory(
	serviceProvider services.Provider,
	sessionFactory SessionFactory,
	endpointService EndpointService,
	identityProvider IdentityProvider,
	registry *Registry,
	tracerProvider tracing.Provider,
	metrics *Metrics,
	localIdentityChecker LocalIdentityChecker,
) ContextFactory {
	return &contextFactory{
		serviceProvider:  serviceProvider,
		sessionFactory:   sessionFactory,
		endpointService:  endpointService,
		identityProvider: identityProvider,
		registry:         registry,
		tracer: tracerProvider.Tracer("calls", tracing.WithMetricsOpts(tracing.MetricsOpts{
			LabelNames: []string{string(SuccessLabel), string(ViewLabel), string(InitiatorViewLabel)},
		})),
		metrics:              metrics,
		localIdentityChecker: localIdentityChecker,
	}
}

func (c *contextFactory) NewForInitiator(
	ctx context.Context,
	contextID string,
	id view.Identity,
	view view.View,
) (ParentContext, error) {
	return NewContextForInitiator(
		contextID,
		ctx,
		c.serviceProvider,
		c.sessionFactory,
		c.endpointService,
		c.identityProvider,
		id,
		view,
		c.tracer,
		c.localIdentityChecker,
	)
}

func (c *contextFactory) NewForResponder(
	ctx context.Context,
	contextID string,
	me view.Identity,
	session view.Session,
	remote view.Identity,
) (ParentContext, error) {
	return NewContext(
		ctx,
		c.serviceProvider,
		contextID,
		c.sessionFactory,
		c.endpointService,
		c.identityProvider,
		me,
		session,
		remote,
		c.tracer,
		c.localIdentityChecker,
	)
}

// Context implements the view.Context interface.
// It provides information about the environment in which a view is in execution.
type Context struct {
	mu             sync.RWMutex
	ctx            context.Context
	sp             services.Provider
	localSP        *ServiceProvider
	id             string
	session        view.Session
	initiator      view.View
	me             view.Identity
	caller         view.Identity
	resolver       EndpointService
	sessionFactory SessionFactory
	idProvider     IdentityProvider

	sessions           *Sessions
	errorCallbackFuncs []func()

	tracer               trace.Tracer
	localIdentityChecker LocalIdentityChecker
}

// NewContextForInitiator returns a new Context for an initiator view.
func NewContextForInitiator(
	contextID string,
	context context.Context,
	sp services.Provider,
	sessionFactory SessionFactory,
	resolver EndpointService,
	idProvider IdentityProvider,
	party view.Identity,
	initiator view.View,
	tracer trace.Tracer,
	localIdentityChecker LocalIdentityChecker,
) (*Context, error) {
	if context == nil {
		return nil, errors.Errorf("a context should not be nil [%s]", string(debug.Stack()))
	}
	if len(contextID) == 0 {
		contextID = utils.GenerateUUID()
	}
	ctx, err := NewContext(
		context,
		sp,
		contextID,
		sessionFactory,
		resolver,
		idProvider,
		party,
		nil,
		nil,
		tracer,
		localIdentityChecker,
	)
	if err != nil {
		return nil, err
	}
	ctx.initiator = initiator

	return ctx, nil
}

// NewContext returns a new Context.
func NewContext(
	ctx context.Context,
	sp services.Provider,
	contextID string,
	sessionFactory SessionFactory,
	resolver EndpointService,
	idProvider IdentityProvider,
	me view.Identity,
	session view.Session,
	caller view.Identity,
	tracer trace.Tracer,
	localIdentityChecker LocalIdentityChecker,
) (*Context, error) {
	if ctx == nil {
		return nil, errors.Errorf("a context should not be nil [%s]", string(debug.Stack()))
	}

	viewCtxt := &Context{
		ctx:                  ctx,
		id:                   contextID,
		resolver:             resolver,
		sessionFactory:       sessionFactory,
		idProvider:           idProvider,
		session:              session,
		me:                   me,
		sessions:             newSessions(),
		caller:               caller,
		sp:                   sp,
		localSP:              NewServiceProvider(),
		localIdentityChecker: localIdentityChecker,

		tracer: tracer,
	}
	if session != nil {
		// Register default session
		viewCtxt.sessions.PutDefault(session.Info().Caller, session)
	}

	return viewCtxt, nil
}

// StartSpan starts a new span from the internal context.
func (c *Context) StartSpan(name string, opts ...trace.SpanStartOption) trace.Span {
	c.mu.Lock()
	defer c.mu.Unlock()
	newCtx, span := c.StartSpanFrom(c.ctx, name, opts...)
	c.ctx = newCtx
	return span
}

// StartSpanFrom creates a new child span from the passed context.
func (c *Context) StartSpanFrom(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return c.tracer.Start(ctx, name, opts...)
}

// ID returns the identifier of this context.
func (c *Context) ID() string {
	return c.id
}

// Initiator returns the View that initiate a call.
func (c *Context) Initiator() view.View {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initiator
}

// RunView runs the passed view on input this context.
func (c *Context) RunView(v view.View, opts ...view.RunViewOption) (res any, err error) {
	return RunViewNow(c, v, opts...)
}

// Me returns the identity bound to this context.
func (c *Context) Me() view.Identity {
	return c.me
}

// Identity returns the identity matching the passed argument.
func (c *Context) Identity(ref string) (view.Identity, error) {
	return c.resolver.GetIdentity(ref, nil)
}

// IsMe returns true if the passed identity is an alias
// of the identity bound to this context, false otherwise.
func (c *Context) IsMe(id view.Identity) bool {
	return c.localIdentityChecker.IsMe(c.Context(), id)
}

// Caller returns the identity of the caller of this context.
func (c *Context) Caller() view.Identity {
	return c.caller
}

// GetSession returns a session to the passed remote party for the given view caller.
// Sessions are scoped by the caller view and cached.
// The session can be bound to other caller views by passing them as additional parameters.
func (c *Context) GetSession(caller view.View, party view.Identity, boundToViews ...view.View) (view.Session, error) {
	viewId := getViewIdentifier(caller)
	logger.DebugfContext(c.Context(), "[%s] GetSession [%s:%s]", c.me, viewId, party)
	// TODO: we need a mechanism to close all the sessions opened in this ctx,
	// when the ctx goes out of scope

	// is there already a session?
	s, targetIdentity := c.sessions.GetFirstOpen(viewId, lazy.NewIterator(
		func() (view.Identity, error) { return party, nil },
		func() (view.Identity, error) { return c.resolve(party) },
		func() (view.Identity, error) { return c.resolve(c.idProvider.Identity(string(party))) },
	))

	// a session is available, return it
	if s != nil {
		logger.DebugfContext(c.Context(), "[%s] Reusing session [%s:%s]", c.me, viewId, party)
		return s, nil
	}
	logger.DebugfContext(c.Context(), "[%s] No session found for [%s:%s], target [%s]", c.me, viewId, party, targetIdentity)

	// create a session
	if caller == nil {
		// return an error, a session should already exist
		return nil, errors.WithMessage(ErrSessionNotFound, "a session should already exist, passed nil view")
	}

	return c.createSession(caller, targetIdentity, boundToViews...)
}

// GetSessionByID returns a session to the passed remote party and id.
func (c *Context) GetSessionByID(id string, party view.Identity) (view.Session, error) {
	// TODO: do we need to resolve?
	var err error
	s := c.sessions.Get(id, party)
	if s == nil {
		logger.DebugfContext(c.Context(), "[%s] Creating new session with given id [id:%s][to:%s]", c.me, id, party)
		s, err = c.newSessionByID(id, c.id, party)
		if err != nil {
			return nil, err
		}
		c.sessions.Put(id, party, s)
	} else {
		logger.DebugfContext(c.Context(), "[%s] Reusing session with given id [id:%s][to:%s]", id, c.me, party)
	}
	return s, nil
}

// Session returns the session created to respond to a remote party,
// nil if the context was created not to respond to a remote call.
func (c *Context) Session() view.Session {
	if c.session == nil {
		logger.DebugfContext(c.Context(), "[%s] No default current Session", c.me)
		return nil
	}
	logger.DebugfContext(c.Context(), "[%s] Current Session [%s]", c.me, logging.Eval(c.session.Info))
	return c.session
}

// ResetSessions disposes all sessions created in this context.
func (c *Context) ResetSessions() error {
	c.sessions.Reset()

	return nil
}

// PutService registers a service in this context.
func (c *Context) PutService(service any) error {
	return c.localSP.RegisterService(service)
}

// GetService returns an instance of the given type.
// It first searches locally in this context, then globally in the service provider.
func (c *Context) GetService(v any) (any, error) {
	// first search locally then globally
	s, err := c.localSP.GetService(v)
	if err == nil {
		return s, nil
	}
	return c.sp.GetService(v)
}

// OnError appends to passed callback function to the list of functions called when
// the current execution return an error or panic.
// This is useful to release resources.
func (c *Context) OnError(callback func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errorCallbackFuncs = append(c.errorCallbackFuncs, callback)
}

// Context returns the associated context.Context.
func (c *Context) Context() context.Context {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ctx
}

// Dispose disposes all sessions created in this context.
func (c *Context) Dispose() {
	logger.DebugfContext(c.Context(), "Dispose sessions")
	// dispose all sessions

	if c.session != nil {
		info := c.session.Info()
		logger.DebugfContext(c.Context(), "Delete one session to %s", string(info.Caller))
		c.sessionFactory.DeleteSessions(c.Context(), info.ID)
	}

	for _, id := range c.sessions.GetSessionIDs() {
		logger.DebugfContext(c.Context(), "Delete session %s", id)
		c.sessionFactory.DeleteSessions(c.Context(), id)
	}
	c.sessions.Reset()
}

func (c *Context) newSession(view view.View, contextID string, party view.Identity) (view.Session, error) {
	resolver, pkid, err := c.resolver.Resolver(c.Context(), party)
	if err != nil {
		return nil, err
	}
	logger.DebugfContext(c.Context(), "Open new session to %s", resolver.GetName())
	return c.sessionFactory.NewSession(GetIdentifier(view), contextID, resolver.GetAddress(endpoint.P2PPort), pkid)
}

func (c *Context) newSessionByID(sessionID, contextID string, party view.Identity) (view.Session, error) {
	resolver, pkid, err := c.resolver.Resolver(c.Context(), party)
	if err != nil {
		return nil, err
	}
	var ep string
	if resolver != nil {
		ep = resolver.GetAddress(endpoint.P2PPort)
		logger.DebugfContext(c.Context(), "Open new session by id to %s", resolver.GetName())
	}
	logger.DebugfContext(c.Context(), "Open new session by id to %s", ep)
	return c.sessionFactory.NewSessionWithID(sessionID, contextID, ep, pkid, nil, nil)
}

// Cleanup calls all error callbacks registered in this context.
func (c *Context) Cleanup() {
	c.mu.RLock()
	logger.DebugfContext(c.ctx, "cleaning up context [%s][%d]", c.id, len(c.errorCallbackFuncs))
	funcs := make([]func(), len(c.errorCallbackFuncs))
	copy(funcs, c.errorCallbackFuncs)
	c.mu.RUnlock()

	for _, callbackFunc := range funcs {
		c.safeInvoke(callbackFunc)
	}
}

func (c *Context) safeInvoke(f func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.DebugfContext(c.ctx, "function [%s] panicked [%s]", f, r)
		}
	}()
	f()
}

func (c *Context) resolve(id view.Identity) (view.Identity, error) {
	if id.IsNone() {
		return nil, errors.WithMessage(ErrInvalidIdentity, "no id provided")
	}
	resolver, _, err := c.resolver.Resolver(c.ctx, id)
	if err != nil {
		return nil, err
	}
	return resolver.GetId(), nil
}

func (c *Context) createSession(caller view.View, party view.Identity, aliases ...view.View) (view.Session, error) {
	logger.DebugfContext(c.ctx, "create session [%s][%s], [%s:%s]", c.me, c.id, getViewIdentifier(caller), party)

	s, err := c.newSession(caller, c.id, party)
	if err != nil {
		return nil, err
	}

	c.sessions.Put(getViewIdentifier(caller), party, s)

	// add aliases as well
	for _, alias := range aliases {
		if alias == nil {
			continue
		}
		c.sessions.Put(getViewIdentifier(alias), party, s)
	}
	return s, nil
}

// PutSessionByID registers a session with the given ID and party in the context.
func (c *Context) PutSessionByID(viewID string, party view.Identity, session view.Session) error {
	c.sessions.Put(viewID, party, session)

	return nil
}

// PutSession registers a session with the given view and party in the context.
func (c *Context) PutSession(caller view.View, party view.Identity, session view.Session) error {
	c.sessions.Put(getViewIdentifier(caller), party, session)

	return nil
}

func getViewIdentifier(f view.View) string {
	if f == nil {
		return ""
	}
	t := reflect.TypeOf(f)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath() + "/" + t.Name()
}
