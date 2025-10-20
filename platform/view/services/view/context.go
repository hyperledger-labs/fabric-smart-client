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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type localContext interface {
	disposableContext
	cleanup()
	PutSession(caller view.View, party view.Identity, session view.Session) error
}

type LocalIdentityChecker interface {
	IsMe(ctx context.Context, id view.Identity) bool
}

//go:generate counterfeiter -o mock/resolver.go -fake-name EndpointService . EndpointService

type EndpointService interface {
	GetIdentity(endpoint string, pkID []byte) (view.Identity, error)
	Resolver(ctx context.Context, party view.Identity) (*endpoint.Resolver, []byte, error)
}

//go:generate counterfeiter -o mock/identity_provider.go -fake-name IdentityProvider . IdentityProvider

type IdentityProvider interface {
	Identity(string) view.Identity
	DefaultIdentity() view.Identity
}

type ctx struct {
	context        context.Context
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
) (*ctx, error) {
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
) (*ctx, error) {
	if context == nil {
		return nil, errors.Errorf("a context should not be nil [%s]", string(debug.Stack()))
	}
	ctx := &ctx{
		context:              context,
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

func (c *ctx) StartSpan(name string, opts ...trace.SpanStartOption) trace.Span {
	newCtx, span := c.StartSpanFrom(c.context, name, opts...)
	c.context = newCtx
	return span
}

func (c *ctx) StartSpanFrom(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return c.tracer.Start(ctx, name, opts...)
}

func (c *ctx) ID() string {
	return c.id
}

func (c *ctx) Initiator() view.View {
	return c.initiator
}

func (c *ctx) RunView(v view.View, opts ...view.RunViewOption) (res interface{}, err error) {
	return runViewOn(v, opts, c)
}

func (c *ctx) Me() view.Identity {
	return c.me
}

// Identity returns the identity matching the passed argument
func (c *ctx) Identity(ref string) (view.Identity, error) {
	return c.resolver.GetIdentity(ref, nil)
}

func (c *ctx) IsMe(id view.Identity) bool {
	return c.localIdentityChecker.IsMe(c.context, id)
}

func (c *ctx) Caller() view.Identity {
	return c.caller
}

func (c *ctx) GetSession(caller view.View, party view.Identity, boundToViews ...view.View) (view.Session, error) {
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
		logger.DebugfContext(c.context, "[%s] Reusing session [%s:%s]", c.me, viewId, party)
		return s, nil
	}

	// create a session
	if caller == nil {
		// return an error, a session should already exist
		return nil, errors.Errorf("a session should already exist, passed nil view")
	}

	return c.createSession(caller, targetIdentity, boundToViews...)
}

func (c *ctx) GetSessionByID(id string, party view.Identity) (view.Session, error) {
	c.sessions.Lock()
	defer c.sessions.Unlock()

	// TODO: do we need to resolve?
	var err error
	s := c.sessions.Get(id, party)
	if s == nil {
		logger.DebugfContext(c.context, "[%s] Creating new session with given id [id:%s][to:%s]", c.me, id, party)
		s, err = c.newSessionByID(id, c.id, party)
		if err != nil {
			return nil, err
		}
		c.sessions.Put(id, party, s)
	} else {
		logger.DebugfContext(c.context, "[%s] Reusing session with given id [id:%s][to:%s]", id, c.me, party)
	}
	return s, nil
}

func (c *ctx) Session() view.Session {
	if c.session == nil {
		logger.DebugfContext(c.context, "[%s] No default current Session", c.me)
		return nil
	}
	logger.DebugfContext(c.context, "[%s] Current Session [%s]", c.me, logging.Eval(c.session.Info))
	return c.session
}

func (c *ctx) ResetSessions() error {
	c.sessions.Lock()
	defer c.sessions.Unlock()
	c.sessions.Reset()

	return nil
}

func (c *ctx) PutService(service interface{}) error {
	return c.localSP.RegisterService(service)
}

func (c *ctx) GetService(v interface{}) (interface{}, error) {
	// first search locally then globally
	s, err := c.localSP.GetService(v)
	if err == nil {
		return s, nil
	}
	return c.sp.GetService(v)
}

func (c *ctx) OnError(callback func()) {
	c.errorCallbackFuncs = append(c.errorCallbackFuncs, callback)
}

func (c *ctx) Context() context.Context {
	return c.context
}

func (c *ctx) Dispose() {
	logger.DebugfContext(c.context, "Dispose sessions")
	// dispose all sessions
	c.sessions.Lock()
	defer c.sessions.Unlock()

	if c.session != nil {
		info := c.session.Info()
		logger.DebugfContext(c.context, "Delete one session to %s", string(info.Caller))
		c.sessionFactory.DeleteSessions(c.Context(), info.ID)
	}

	for _, id := range c.sessions.GetSessionIDs() {
		logger.DebugfContext(c.context, "Delete session %s", id)
		c.sessionFactory.DeleteSessions(c.Context(), id)
	}
	c.sessions.Reset()
}

func (c *ctx) newSession(view view.View, contextID string, party view.Identity) (view.Session, error) {
	resolver, pkid, err := c.resolver.Resolver(c.context, party)
	if err != nil {
		return nil, err
	}
	logger.DebugfContext(c.context, "Open new session to %s", resolver.GetName())
	return c.sessionFactory.NewSession(GetIdentifier(view), contextID, resolver.GetAddress(endpoint.P2PPort), pkid)
}

func (c *ctx) newSessionByID(sessionID, contextID string, party view.Identity) (view.Session, error) {
	resolver, pkid, err := c.resolver.Resolver(c.context, party)
	if err != nil {
		return nil, err
	}
	var ep string
	if resolver != nil {
		ep = resolver.GetAddress(endpoint.P2PPort)
		logger.DebugfContext(c.context, "Open new session by id to %s", resolver.GetName())
	}
	logger.DebugfContext(c.context, "Open new session by id to %s", ep)
	return c.sessionFactory.NewSessionWithID(sessionID, contextID, ep, pkid, nil, nil)
}

func (c *ctx) cleanup() {
	logger.DebugfContext(c.context, "cleaning up context [%s][%d]", c.ID(), len(c.errorCallbackFuncs))
	for _, callbackFunc := range c.errorCallbackFuncs {
		c.safeInvoke(callbackFunc)
	}
}

func (c *ctx) safeInvoke(f func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.DebugfContext(c.context, "function [%s] panicked [%s]", f, r)
		}
	}()
	f()
}

func (c *ctx) resolve(id view.Identity) (view.Identity, error) {
	if id.IsNone() {
		return nil, errors.New("no id provided")
	}
	resolver, _, err := c.resolver.Resolver(c.context, id)
	if err != nil {
		return nil, err
	}
	return resolver.GetId(), nil
}

func (c *ctx) createSession(caller view.View, party view.Identity, aliases ...view.View) (view.Session, error) {
	logger.DebugfContext(c.context, "create session [%s][%s], [%s:%s]", c.me, c.id, getViewIdentifier(caller), party)

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

func (c *ctx) PutSession(caller view.View, party view.Identity, session view.Session) error {
	c.sessions.Lock()
	defer c.sessions.Unlock()

	c.sessions.Put(getViewIdentifier(caller), party, session)

	return nil
}

type tempCtx struct {
	localContext
	newCtx context.Context
}

func (c *tempCtx) Context() context.Context {
	return c.newCtx
}

func runViewOn(v view.View, opts []view.RunViewOption, ctx localContext) (res interface{}, err error) {
	options, err := view.CompileRunViewOptions(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed compiling options")
	}
	var initiator view.View
	if options.AsInitiator {
		initiator = v
	}

	logger.DebugfContext(ctx.Context(), "Start view %s", GetName(v))
	newCtx, span := ctx.StartSpanFrom(ctx.Context(), GetName(v), tracing.WithAttributes(
		tracing.String(ViewLabel, GetIdentifier(v)),
		tracing.String(InitiatorViewLabel, GetIdentifier(initiator)),
	), trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	var cc localContext
	if options.SameContext {
		cc = wrapContext(ctx, newCtx)
	} else {
		if options.AsInitiator {
			cc = &childContext{
				ParentContext: wrapContext(ctx, newCtx),
				initiator:     initiator,
			}
			// register options.Session under initiator
			contextSession := ctx.Session()
			if contextSession == nil {
				return nil, errors.Errorf("cannot convert a non-responder context to an initiator context")
			}
			if err := cc.PutSession(initiator, contextSession.Info().Caller, contextSession); err != nil {
				return nil, errors.Wrapf(err, "failed registering default session as initiated by [%s:%s]", initiator, contextSession.Info().Caller)
			}
		} else {
			cc = &childContext{
				ParentContext: wrapContext(ctx, newCtx),
				session:       options.Session,
				initiator:     initiator,
			}
		}
	}

	defer func() {
		if r := recover(); r != nil {
			cc.cleanup()
			res = nil

			logger.Errorf("caught panic while running view with [%v][%s]", r, debug.Stack())

			switch e := r.(type) {
			case error:
				err = errors.WithMessage(e, "caught panic")
			case string:
				err = errors.New(e)
			default:
				err = errors.Errorf("caught panic [%v]", e)
			}
		}
	}()

	if v == nil && options.Call == nil {
		return nil, errors.Errorf("no view passed")
	}
	if options.Call != nil {
		res, err = options.Call(cc)
	} else {
		res, err = v.Call(cc)
	}
	span.SetAttributes(attribute.Bool(SuccessLabel, err != nil))
	if err != nil {
		cc.cleanup()
		return nil, err
	}
	return res, err
}

func wrapContext(ctx localContext, newCtx context.Context) localContext {
	return &tempCtx{
		localContext: ctx,
		newCtx:       newCtx,
	}
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
