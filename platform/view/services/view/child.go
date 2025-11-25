/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.opentelemetry.io/otel/trace"
)

//go:generate counterfeiter -o mock/parent_context.go -fake-name ParentContext . ParentContext
type ParentContext interface {
	DisposableContext
	Cleanup()
	PutSession(caller view.View, party view.Identity, session view.Session) error
}

// ChildContext is a view context with a parent.
// It allows the developer to override session and initiator.
// It also supports error callbacks.
type ChildContext struct {
	Parent ParentContext

	session            view.Session
	initiator          view.View
	errorCallbackFuncs []func()
}

// NewChildContextFromParent return a new ChildContext from the given parent
func NewChildContextFromParent(parentContext ParentContext) *ChildContext {
	return NewChildContext(parentContext, nil, nil, nil)
}

// NewChildContextFromParentAndSession return a new ChildContext from the given parent and session
func NewChildContextFromParentAndSession(parentContext ParentContext, session view.Session) *ChildContext {
	return NewChildContext(parentContext, session, nil, nil)
}

// NewChildContextFromParentAndInitiator return a new ChildContext from the given parent and initiator view
func NewChildContextFromParentAndInitiator(parentContext ParentContext, initiator view.View) *ChildContext {
	return NewChildContext(parentContext, nil, initiator, nil)
}

// NewChildContext return a new ChildContext from the given arguments
func NewChildContext(parentContext ParentContext, session view.Session, initiator view.View, errorCallbackFuncs ...func()) *ChildContext {
	return &ChildContext{Parent: parentContext, session: session, initiator: initiator, errorCallbackFuncs: errorCallbackFuncs}
}

func (w *ChildContext) StartSpanFrom(c context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return w.Parent.StartSpanFrom(c, name, opts...)
}

func (w *ChildContext) GetService(v interface{}) (interface{}, error) {
	return w.Parent.GetService(v)
}

func (w *ChildContext) PutService(v interface{}) error {
	mutableContext, ok := w.Parent.(view.MutableContext)
	if ok {
		return mutableContext.PutService(v)
	}
	return nil
}

func (w *ChildContext) ID() string {
	return w.Parent.ID()
}

func (w *ChildContext) Me() view.Identity {
	return w.Parent.Me()
}

func (w *ChildContext) IsMe(id view.Identity) bool {
	return w.Parent.IsMe(id)
}

func (w *ChildContext) GetSession(caller view.View, party view.Identity, boundToViews ...view.View) (view.Session, error) {
	return w.Parent.GetSession(caller, party, boundToViews...)
}

func (w *ChildContext) GetSessionByID(id string, party view.Identity) (view.Session, error) {
	return w.Parent.GetSessionByID(id, party)
}

func (w *ChildContext) Context() context.Context {
	return w.Parent.Context()
}

func (w *ChildContext) Session() view.Session {
	if w.session == nil {
		return w.Parent.Session()
	}
	return w.session
}

func (w *ChildContext) ResetSessions() error {
	mutableContext, ok := w.Parent.(view.MutableContext)
	if ok {
		return mutableContext.ResetSessions()
	}
	return nil
}

func (w *ChildContext) Initiator() view.View {
	if w.initiator == nil {
		return w.Parent.Initiator()
	}
	return w.initiator
}

func (w *ChildContext) OnError(f func()) {
	w.errorCallbackFuncs = append(w.errorCallbackFuncs, f)
}

func (w *ChildContext) RunView(v view.View, opts ...view.RunViewOption) (res interface{}, err error) {
	return RunViewNow(w, v, opts...)
}

func (w *ChildContext) Dispose() {
	if w.Parent != nil {
		w.Parent.Dispose()
	}
}

func (w *ChildContext) PutSession(caller view.View, party view.Identity, session view.Session) error {
	return w.Parent.PutSession(caller, party, session)
}

func (w *ChildContext) Cleanup() {
	logger.Debugf("cleaning up child context [%s][%d]", w.ID(), len(w.errorCallbackFuncs))
	for _, callbackFunc := range w.errorCallbackFuncs {
		w.safeInvoke(callbackFunc)
	}
}

func (w *ChildContext) safeInvoke(f func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.Debugf("function [%s] panicked [%s]", f, r)
		}
	}()
	f()
}
