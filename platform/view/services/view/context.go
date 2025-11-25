/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"reflect"
	"runtime/debug"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.opentelemetry.io/otel/trace"
)

// LocalIdentityChecker models the dependency to the view-sdk's sig service
type LocalIdentityChecker interface {
	IsMe(ctx context.Context, id view.Identity) bool
}

// EndpointService models the dependency to the view-sdk's endpoint service
//
//go:generate counterfeiter -o mock/resolver.go -fake-name EndpointService . EndpointService
type EndpointService interface {
	GetIdentity(endpoint string, pkID []byte) (view.Identity, error)
	Resolver(ctx context.Context, party view.Identity) (*endpoint.Resolver, []byte, error)
}

// IdentityProvider models the dependency to the view-sdk's identity provider
//
//go:generate counterfeiter -o mock/identity_provider.go -fake-name IdentityProvider . IdentityProvider
type IdentityProvider interface {
	Identity(string) view.Identity
	DefaultIdentity() view.Identity
}

// Context implements the view.Context interface
type Context struct {
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

// NewContextForInitiator returns a new Context for an initiator view
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

// NewContext returns a new Context
func NewContext(
	context context.Context,
	sp services.Provider,
	contextID string,
	sessionFactory SessionFactory,
	resolver EndpointService,
	idProvider IdentityProvider,
	party view.Identity,
	session view.Session,
	caller view.Identity,
	tracer trace.Tracer,
	localIdentityChecker LocalIdentityChecker,
) (*Context, error) {
	if context == nil {
		return nil, errors.Errorf("a context should not be nil [%s]", string(debug.Stack()))
	}
	ctx := &Context{
		ctx:                  context,
		id:                   contextID,
		resolver:             resolver,
		sessionFactory:       sessionFactory,
		idProvider:           idProvider,
		session:              session,
		me:                   party,
		sessions:             newSessions(),
		caller:               caller,
		sp:                   sp,
		localSP:              NewServiceProvider(),
		localIdentityChecker: localIdentityChecker,

		tracer: tracer,
	}
	if session != nil {
		// Register default session
		ctx.sessions.PutDefault(session.Info().Caller, session)
	}

	return ctx, nil
}

func (c *Context) StartSpan(name string, opts ...trace.SpanStartOption) trace.Span {
	newCtx, span := c.StartSpanFrom(c.ctx, name, opts...)
	c.ctx = newCtx
	return span
}

func (c *Context) StartSpanFrom(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return c.tracer.Start(ctx, name, opts...)
}

func (c *Context) ID() string {
	return c.id
}

func (c *Context) Initiator() view.View {
	return c.initiator
}

func (c *Context) RunView(v view.View, opts ...view.RunViewOption) (res interface{}, err error) {
	return RunViewNow(c, v, opts...)
}

func (c *Context) Me() view.Identity {
	return c.me
}

// Identity returns the identity matching the passed argument
func (c *Context) Identity(ref string) (view.Identity, error) {
	return c.resolver.GetIdentity(ref, nil)
}

func (c *Context) IsMe(id view.Identity) bool {
	return c.localIdentityChecker.IsMe(c.ctx, id)
}

func (c *Context) Caller() view.Identity {
	return c.caller
}

func (c *Context) GetSession(caller view.View, party view.Identity, boundToViews ...view.View) (view.Session, error) {
	viewId := getViewIdentifier(caller)
	// TODO: we need a mechanism to close all the sessions opened in this ctx,
	// when the ctx goes out of scope
	c.sessions.Lock()
	defer c.sessions.Unlock()

	// is there already a session?
	s, targetIdentity := c.sessions.GetFirstOpen(viewId, lazy.NewIterator(
		func() (view.Identity, error) { return party, nil },
		func() (view.Identity, error) { return c.resolve(party) },
		func() (view.Identity, error) { return c.resolve(c.idProvider.Identity(string(party))) },
	))

	// a session is available, return it
	if s != nil {
		logger.DebugfContext(c.ctx, "[%s] Reusing session [%s:%s]", c.me, viewId, party)
		return s, nil
	}

	// create a session
	if caller == nil {
		// return an error, a session should already exist
		return nil, errors.Errorf("a session should already exist, passed nil view")
	}

	return c.createSession(caller, targetIdentity, boundToViews...)
}

func (c *Context) GetSessionByID(id string, party view.Identity) (view.Session, error) {
	c.sessions.Lock()
	defer c.sessions.Unlock()

	// TODO: do we need to resolve?
	var err error
	s := c.sessions.Get(id, party)
	if s == nil {
		logger.DebugfContext(c.ctx, "[%s] Creating new session with given id [id:%s][to:%s]", c.me, id, party)
		s, err = c.newSessionByID(id, c.id, party)
		if err != nil {
			return nil, err
		}
		c.sessions.Put(id, party, s)
	} else {
		logger.DebugfContext(c.ctx, "[%s] Reusing session with given id [id:%s][to:%s]", id, c.me, party)
	}
	return s, nil
}

func (c *Context) Session() view.Session {
	if c.session == nil {
		logger.DebugfContext(c.ctx, "[%s] No default current Session", c.me)
		return nil
	}
	logger.DebugfContext(c.ctx, "[%s] Current Session [%s]", c.me, logging.Eval(c.session.Info))
	return c.session
}

func (c *Context) ResetSessions() error {
	c.sessions.Lock()
	defer c.sessions.Unlock()
	c.sessions.Reset()

	return nil
}

func (c *Context) PutService(service interface{}) error {
	return c.localSP.RegisterService(service)
}

func (c *Context) GetService(v interface{}) (interface{}, error) {
	// first search locally then globally
	s, err := c.localSP.GetService(v)
	if err == nil {
		return s, nil
	}
	return c.sp.GetService(v)
}

func (c *Context) OnError(callback func()) {
	c.errorCallbackFuncs = append(c.errorCallbackFuncs, callback)
}

func (c *Context) Context() context.Context {
	return c.ctx
}

func (c *Context) Dispose() {
	logger.DebugfContext(c.ctx, "Dispose sessions")
	// dispose all sessions
	c.sessions.Lock()
	defer c.sessions.Unlock()

	if c.session != nil {
		info := c.session.Info()
		logger.DebugfContext(c.ctx, "Delete one session to %s", string(info.Caller))
		c.sessionFactory.DeleteSessions(c.Context(), info.ID)
	}

	for _, id := range c.sessions.GetSessionIDs() {
		logger.DebugfContext(c.ctx, "Delete session %s", id)
		c.sessionFactory.DeleteSessions(c.Context(), id)
	}
	c.sessions.Reset()
}

func (c *Context) newSession(view view.View, contextID string, party view.Identity) (view.Session, error) {
	resolver, pkid, err := c.resolver.Resolver(c.ctx, party)
	if err != nil {
		return nil, err
	}
	logger.DebugfContext(c.ctx, "Open new session to %s", resolver.GetName())
	return c.sessionFactory.NewSession(GetIdentifier(view), contextID, resolver.GetAddress(endpoint.P2PPort), pkid)
}

func (c *Context) newSessionByID(sessionID, contextID string, party view.Identity) (view.Session, error) {
	resolver, pkid, err := c.resolver.Resolver(c.ctx, party)
	if err != nil {
		return nil, err
	}
	var ep string
	if resolver != nil {
		ep = resolver.GetAddress(endpoint.P2PPort)
		logger.DebugfContext(c.ctx, "Open new session by id to %s", resolver.GetName())
	}
	logger.DebugfContext(c.ctx, "Open new session by id to %s", ep)
	return c.sessionFactory.NewSessionWithID(sessionID, contextID, ep, pkid, nil, nil)
}

func (c *Context) Cleanup() {
	logger.DebugfContext(c.ctx, "cleaning up context [%s][%d]", c.ID(), len(c.errorCallbackFuncs))
	for _, callbackFunc := range c.errorCallbackFuncs {
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
		return nil, errors.New("no id provided")
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

func (c *Context) PutSession(caller view.View, party view.Identity, session view.Session) error {
	c.sessions.Lock()
	defer c.sessions.Unlock()

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
