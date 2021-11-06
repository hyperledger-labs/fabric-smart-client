/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ctx struct {
	context        context.Context
	sp             driver.ServiceProvider
	localSP        driver.ServiceProvider
	id             string
	session        view.Session
	initiator      view.View
	me             view.Identity
	caller         view.Identity
	resolver       driver.EndpointService
	sessionFactory SessionFactory

	sessionsLock       sync.RWMutex
	sessions           map[string]view.Session
	errorCallbackFuncs []func()
}

func NewContextForInitiator(context context.Context, sp driver.ServiceProvider, sessionFactory SessionFactory, resolver driver.EndpointService, party view.Identity, initiator view.View) (*ctx, error) {
	ctx, err := NewContext(context, sp, GenerateUUID(), sessionFactory, resolver, party, nil, nil)
	if err != nil {
		return nil, err
	}
	ctx.initiator = initiator

	return ctx, nil
}

func NewContext(context context.Context, sp driver.ServiceProvider, contextID string, sessionFactory SessionFactory, resolver driver.EndpointService, party view.Identity, session view.Session, caller view.Identity) (*ctx, error) {
	ctx := &ctx{
		context:        context,
		id:             contextID,
		resolver:       resolver,
		sessionFactory: sessionFactory,
		session:        session,
		me:             party,
		sessions:       map[string]view.Session{},
		caller:         caller,
		sp:             sp,
		localSP:        registry.New(),
	}
	if session != nil {
		// Register default session
		ctx.sessions[session.Info().Caller.UniqueID()] = session
	}

	return ctx, nil
}

func (ctx *ctx) ID() string {
	return ctx.id
}

func (ctx *ctx) Initiator() view.View {
	return ctx.initiator
}

func (ctx *ctx) RunView(v view.View, opts ...view.RunViewOption) (res interface{}, err error) {
	options, err := view.CompileRunViewOptions(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed compiling options")
	}
	var initiator view.View
	if options.AsInitiator {
		initiator = v
	}

	var cc localContext
	if options.SameContext {
		cc = ctx
	} else {
		cc = &childContext{
			ParentContext: ctx,
			session:       options.Session,
			initiator:     initiator,
		}
	}

	defer func() {
		if r := recover(); r != nil {
			cc.cleanup()
			res = nil

			logger.Warningf("caught panic while running view with [%v][%s]", r, debug.Stack())

			switch e := r.(type) {
			case error:
				err = errors.WithMessage(e, "caught panic")
			case string:
				err = errors.Errorf(e)
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
	if err != nil {
		cc.cleanup()
		return nil, err
	}
	return res, err
}

func (ctx *ctx) Me() view.Identity {
	return ctx.me
}

// TODO: remove this
func (ctx *ctx) Identity(ref string) (view.Identity, error) {
	return driver.GetEndpointService(ctx.sp).GetIdentity(ref, nil)
}

func (ctx *ctx) IsMe(id view.Identity) bool {
	return view2.GetSigService(ctx).IsMe(id)
}

func (ctx *ctx) Caller() view.Identity {
	return ctx.caller
}

func (ctx *ctx) GetSession(f view.View, party view.Identity) (view.Session, error) {
	// TODO: we need a mechanism to close all the sessions opened in this ctx,
	// when the ctx goes out of scope
	ctx.sessionsLock.Lock()
	defer ctx.sessionsLock.Unlock()

	var err error
	id := party

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("get session for [%s:%s]", id.UniqueID(), getIdentifier(f))
	}
	s, ok := ctx.sessions[id.UniqueID()]
	if !ok {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("session for [%s] does not exists, resolve", id.UniqueID())
		}
		id, _, _, err = view2.GetEndpointService(ctx).Resolve(party)
		if err == nil {
			s, ok = ctx.sessions[id.UniqueID()]
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("session resolved for [%s] exists? [%v]", id.UniqueID(), ok)
			}
		}
	} else {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("session for [%s] found", id.UniqueID())
		}
	}

	if ok && s.Info().Closed {
		// Remove this session cause it is closed
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("removing session [%s], it is closed", id.UniqueID(), ok)
		}
		delete(ctx.sessions, id.UniqueID())
		ok = false
	}

	if !ok {
		if f == nil {
			// return an error, a session should already exist
			return nil, errors.Errorf("a session should already exist, passed nil view")
		}

		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("[%s] Creating new session [to:%s]", ctx.me, id)
		}
		s, err = ctx.newSession(f, ctx.id, id)
		if err != nil {
			return nil, err
		}
		ctx.sessions[id.UniqueID()] = s
	} else {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("[%s] Reusing session [to:%s]", ctx.me, id)
		}
	}
	return s, nil
}

func (ctx *ctx) GetSessionByID(id string, party view.Identity) (view.Session, error) {
	ctx.sessionsLock.Lock()
	defer ctx.sessionsLock.Unlock()

	// TODO: do we need to resolve?
	var err error
	key := id + "." + party.UniqueID()
	s, ok := ctx.sessions[key]
	if !ok {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("[%s] Creating new session with given id [id:%s][to:%s]", ctx.me, id, party)
		}
		s, err = ctx.newSessionByID(id, ctx.id, party)
		if err != nil {
			return nil, err
		}
		ctx.sessions[key] = s
	} else {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("[%s] Reusing session with given id [id:%s][to:%s]", id, ctx.me, party)
		}
	}
	return s, nil
}

func (ctx *ctx) Session() view.Session {
	if ctx.session == nil {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("[%s] No default current Session", ctx.me)
		}
		return nil
	}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[%s] Current Session [%s]", ctx.me, ctx.session.Info())
	}
	return ctx.session
}

func (ctx *ctx) ResetSessions() error {
	ctx.sessionsLock.Lock()
	defer ctx.sessionsLock.Unlock()
	ctx.sessions = map[string]view.Session{}

	return nil
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
	// dispose all sessions
	ctx.sessionsLock.Lock()
	defer ctx.sessionsLock.Unlock()

	if ctx.session != nil {
		ctx.sessionFactory.DeleteSessions(ctx.session.Info().ID)
	}

	for _, s := range ctx.sessions {
		ctx.sessionFactory.DeleteSessions(s.Info().ID)
	}
	ctx.sessions = map[string]view.Session{}
}

func (ctx *ctx) newSession(view view.View, contextID string, party view.Identity) (view.Session, error) {
	_, endpoints, pkid, err := ctx.resolver.Resolve(party)
	if err != nil {
		return nil, err
	}
	return ctx.sessionFactory.NewSession(getIdentifier(view), contextID, endpoints[driver.P2PPort], pkid)
}

func (ctx *ctx) newSessionByID(sessionID, contextID string, party view.Identity) (view.Session, error) {
	_, endpoints, pkid, err := ctx.resolver.Resolve(party)
	if err != nil {
		return nil, err
	}
	return ctx.sessionFactory.NewSessionWithID(sessionID, contextID, endpoints[driver.P2PPort], pkid, nil, nil)
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
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("function [%s] panicked [%s]", f, r)
			}
		}
	}()
	f()
}

type localContext interface {
	view.Context
	cleanup()
}
