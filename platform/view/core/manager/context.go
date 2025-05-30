/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager

import (
	"context"
	"fmt"
	"reflect"
	"runtime/debug"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/registry"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type localContext interface {
	disposableContext
	cleanup()
	PutSession(caller view.View, party view.Identity, session view.Session) error
}

type ctx struct {
	context        context.Context
	sp             driver.ServiceProvider
	localSP        *registry.ServiceProvider
	id             string
	session        view.Session
	initiator      view.View
	me             view.Identity
	caller         view.Identity
	resolver       driver.EndpointService
	sessionFactory SessionFactory
	idProvider     driver.IdentityProvider

	sessions           *Sessions
	errorCallbackFuncs []func()

	tracer trace.Tracer
}

func NewContextForInitiator(contextID string, context context.Context, sp driver.ServiceProvider, sessionFactory SessionFactory, resolver driver.EndpointService, idProvider driver.IdentityProvider, party view.Identity, initiator view.View, tracer trace.Tracer) (*ctx, error) {
	if len(contextID) == 0 {
		contextID = GenerateUUID()
	}
	ctx, err := NewContext(context, sp, contextID, sessionFactory, resolver, idProvider, party, nil, nil, tracer)
	if err != nil {
		return nil, err
	}
	ctx.initiator = initiator

	return ctx, nil
}

func NewContext(context context.Context, sp driver.ServiceProvider, contextID string, sessionFactory SessionFactory, resolver driver.EndpointService, idProvider driver.IdentityProvider, party view.Identity, session view.Session, caller view.Identity, tracer trace.Tracer) (*ctx, error) {
	ctx := &ctx{
		context:        context,
		id:             contextID,
		resolver:       resolver,
		sessionFactory: sessionFactory,
		idProvider:     idProvider,
		session:        session,
		me:             party,
		sessions:       newSessions(),
		caller:         caller,
		sp:             sp,
		localSP:        registry.New(),

		tracer: tracer,
	}
	if session != nil {
		// Register default session
		ctx.sessions.PutDefault(session.Info().Caller, session)
	}

	return ctx, nil
}

func (ctx *ctx) StartSpan(name string, opts ...trace.SpanStartOption) trace.Span {
	newCtx, span := ctx.StartSpanFrom(ctx.context, name, opts...)
	ctx.context = newCtx
	return span
}

func (ctx *ctx) StartSpanFrom(c context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx.tracer.Start(c, name, opts...)
}

func (ctx *ctx) ID() string {
	return ctx.id
}

func (ctx *ctx) Initiator() view.View {
	return ctx.initiator
}

func (ctx *ctx) RunView(v view.View, opts ...view.RunViewOption) (res interface{}, err error) {
	return runViewOn(v, opts, ctx)
}

func (ctx *ctx) Me() view.Identity {
	return ctx.me
}

// TODO: remove this
func (ctx *ctx) Identity(ref string) (view.Identity, error) {
	return ctx.resolver.GetIdentity(ref, nil)
}

func (ctx *ctx) IsMe(id view.Identity) bool {
	return view2.GetSigService(ctx).IsMe(ctx.context, id)
}

func (ctx *ctx) Caller() view.Identity {
	return ctx.caller
}

func (ctx *ctx) GetSession(caller view.View, party view.Identity, boundToViews ...view.View) (view.Session, error) {
	viewId := getViewIdentifier(caller)
	// TODO: we need a mechanism to close all the sessions opened in this ctx,
	// when the ctx goes out of scope
	ctx.sessions.Lock()
	defer ctx.sessions.Unlock()

	// is there already a session?
	s, targetIdentity := ctx.sessions.GetFirstOpen(viewId, lazy.NewIterator(
		func() (view.Identity, error) { return party, nil },
		func() (view.Identity, error) { return ctx.resolve(party) },
		func() (view.Identity, error) { return ctx.resolve(ctx.idProvider.Identity(string(party))) },
	))

	// a session is available, return it
	if s != nil {
		logger.Debugf("[%s] Reusing session [%s:%s]", ctx.me, viewId, party)
		return s, nil
	}

	// create a session
	if caller == nil {
		// return an error, a session should already exist
		return nil, errors.Errorf("a session should already exist, passed nil view")
	}

	return ctx.createSession(caller, targetIdentity, boundToViews...)
}

func (ctx *ctx) GetSessionByID(id string, party view.Identity) (view.Session, error) {
	ctx.sessions.Lock()
	defer ctx.sessions.Unlock()

	// TODO: do we need to resolve?
	var err error
	s := ctx.sessions.Get(id, party)
	if s == nil {
		logger.Debugf("[%s] Creating new session with given id [id:%s][to:%s]", ctx.me, id, party)
		s, err = ctx.newSessionByID(id, ctx.id, party)
		if err != nil {
			return nil, err
		}
		ctx.sessions.Put(id, party, s)
	} else {
		span := trace.SpanFromContext(ctx.context)
		span.AddEvent(fmt.Sprintf("Reuse session to %s", string(party)))
		logger.Debugf("[%s] Reusing session with given id [id:%s][to:%s]", id, ctx.me, party)
	}
	return s, nil
}

func (ctx *ctx) Session() view.Session {
	if ctx.session == nil {
		logger.Debugf("[%s] No default current Session", ctx.me)
		return nil
	}
	logger.Debugf("[%s] Current Session [%s]", ctx.me, logging.Eval(ctx.session.Info))
	return ctx.session
}

func (ctx *ctx) ResetSessions() error {
	ctx.sessions.Lock()
	defer ctx.sessions.Unlock()
	ctx.sessions.Reset()

	return nil
}

func (ctx *ctx) PutService(service interface{}) error {
	return ctx.localSP.RegisterService(service)
}

func (ctx *ctx) GetService(v interface{}) (interface{}, error) {
	// first search locally then globally
	s, err := ctx.localSP.GetService(v)
	if err == nil {
		return s, nil
	}
	return ctx.sp.GetService(v)
}

func (ctx *ctx) OnError(callback func()) {
	ctx.errorCallbackFuncs = append(ctx.errorCallbackFuncs, callback)
}

func (ctx *ctx) Context() context.Context {
	return ctx.context
}

func (ctx *ctx) Dispose() {
	span := trace.SpanFromContext(ctx.context)
	span.AddEvent("Dispose sessions")
	// dispose all sessions
	ctx.sessions.Lock()
	defer ctx.sessions.Unlock()

	if ctx.session != nil {
		span.AddEvent(fmt.Sprintf("Delete one session to %s", string(ctx.session.Info().Caller)))
		ctx.sessionFactory.DeleteSessions(ctx.Context(), ctx.session.Info().ID)
	}

	for _, id := range ctx.sessions.GetSessionIDs() {
		span.AddEvent(fmt.Sprintf("Delete session %s", id))
		ctx.sessionFactory.DeleteSessions(ctx.Context(), id)
	}
	ctx.sessions.Reset()
}

func (ctx *ctx) newSession(view view.View, contextID string, party view.Identity) (view.Session, error) {
	span := trace.SpanFromContext(ctx.context)
	resolver, pkid, err := ctx.resolver.Resolve(ctx.context, party)
	if err != nil {
		return nil, err
	}
	span.AddEvent(fmt.Sprintf("Open new session to %s", resolver.GetName()))
	return ctx.sessionFactory.NewSession(registry2.GetIdentifier(view), contextID, resolver.GetAddress(driver.P2PPort), pkid)
}

func (ctx *ctx) newSessionByID(sessionID, contextID string, party view.Identity) (view.Session, error) {
	span := trace.SpanFromContext(ctx.context)
	resolver, pkid, err := ctx.resolver.Resolve(ctx.context, party)
	if err != nil {
		return nil, err
	}
	var endpoint string
	if resolver != nil {
		endpoint = resolver.GetAddress(driver.P2PPort)
		span.AddEvent(fmt.Sprintf("Open new session by id to %s", resolver.GetName()))
	}
	span.AddEvent(fmt.Sprintf("Open new session by id to %s", endpoint))
	return ctx.sessionFactory.NewSessionWithID(sessionID, contextID, endpoint, pkid, nil, nil)
}

func (ctx *ctx) cleanup() {
	logger.Debugf("cleaning up context [%s][%d]", ctx.ID(), len(ctx.errorCallbackFuncs))
	for _, callbackFunc := range ctx.errorCallbackFuncs {
		ctx.safeInvoke(callbackFunc)
	}
}

func (ctx *ctx) safeInvoke(f func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.Debugf("function [%s] panicked [%s]", f, r)
		}
	}()
	f()
}

func (ctx *ctx) resolve(id view.Identity) (view.Identity, error) {
	if id.IsNone() {
		return nil, errors.New("no id provided")
	}
	resolver, _, err := ctx.resolver.Resolve(ctx.context, id)
	if err != nil {
		return nil, err
	}
	return resolver.GetId(), nil
}

func (ctx *ctx) createSession(caller view.View, party view.Identity, aliases ...view.View) (view.Session, error) {
	logger.Infof("create session [%s][%s], [%s:%s]", ctx.me, ctx.id, getViewIdentifier(caller), party)

	s, err := ctx.newSession(caller, ctx.id, party)
	if err != nil {
		return nil, err
	}

	ctx.sessions.Put(getViewIdentifier(caller), party, s)

	// add aliases as well
	for _, alias := range aliases {
		if alias == nil {
			continue
		}
		ctx.sessions.Put(getViewIdentifier(alias), party, s)
	}
	return s, nil
}

func (ctx *ctx) PutSession(caller view.View, party view.Identity, session view.Session) error {
	ctx.sessions.Lock()
	defer ctx.sessions.Unlock()

	ctx.sessions.Put(getViewIdentifier(caller), party, session)

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

	newCtx, span := ctx.StartSpanFrom(ctx.Context(), registry2.GetName(v), tracing.WithAttributes(
		tracing.String(ViewLabel, registry2.GetIdentifier(v)),
		tracing.String(InitiatorViewLabel, registry2.GetIdentifier(initiator)),
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
