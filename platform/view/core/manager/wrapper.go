/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
)

type disposableContext interface {
	view.Context
	Dispose()
}

type childContext struct {
	ParentContext localContext

	session            view.Session
	initiator          view.View
	errorCallbackFuncs []func()
}

func (w *childContext) StartSpanFrom(c context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return w.ParentContext.StartSpanFrom(c, name, opts...)
}

func (w *childContext) StartSpan(name string, opts ...trace.SpanStartOption) trace.Span {
	return w.ParentContext.StartSpan(name, opts...)
}

func (w *childContext) GetService(v interface{}) (interface{}, error) {
	return w.ParentContext.GetService(v)
}

func (w *childContext) PutService(v interface{}) error {
	mutableContext, ok := w.ParentContext.(view.MutableContext)
	if ok {
		return mutableContext.PutService(v)
	}
	return nil
}

func (w *childContext) ID() string {
	return w.ParentContext.ID()
}

func (w *childContext) Me() view.Identity {
	return w.ParentContext.Me()
}

func (w *childContext) IsMe(id view.Identity) bool {
	return w.ParentContext.IsMe(id)
}

func (w *childContext) GetSession(caller view.View, party view.Identity) (view.Session, error) {
	return w.ParentContext.GetSession(caller, party)
}

func (w *childContext) GetSessionByID(id string, party view.Identity) (view.Session, error) {
	return w.ParentContext.GetSessionByID(id, party)
}

func (w *childContext) Context() context.Context {
	return w.ParentContext.Context()
}

func (w *childContext) Session() view.Session {
	if w.session == nil {
		return w.ParentContext.Session()
	}
	return w.session
}

func (w *childContext) ResetSessions() error {
	mutableContext, ok := w.ParentContext.(view.MutableContext)
	if ok {
		return mutableContext.ResetSessions()
	}
	return nil
}

func (w *childContext) Initiator() view.View {
	if w.initiator == nil {
		return w.ParentContext.Initiator()
	}
	return w.initiator
}

func (w *childContext) OnError(f func()) {
	w.errorCallbackFuncs = append(w.errorCallbackFuncs, f)
}

func (w *childContext) RunView(v view.View, opts ...view.RunViewOption) (res interface{}, err error) {
	return runViewOn(v, opts, w)
}

func (w *childContext) Dispose() {
	if w.ParentContext != nil {
		w.ParentContext.Dispose()
	}
}

func (w *childContext) cleanup() {
	logger.Debugf("cleaning up child context [%s][%d]", w.ID(), len(w.errorCallbackFuncs))
	for _, callbackFunc := range w.errorCallbackFuncs {
		w.safeInvoke(callbackFunc)
	}
}

func (w *childContext) safeInvoke(f func()) {
	defer func() {
		if r := recover(); r != nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("function [%s] panicked [%s]", f, r)
			}
		}
	}()
	f()
}
