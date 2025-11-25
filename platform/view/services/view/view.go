/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"runtime/debug"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ViewContext is an alias for view.Context
//
//go:generate counterfeiter -o mock/context.go -fake-name Context . ViewContext
type ViewContext = view.Context

// View is an alias for View
//
//go:generate counterfeiter -o mock/view.go -fake-name View . View
type View = view.View

// RunCall is a shortcut for `context.RunView(nil, view.WithViewCall(v))` and can be used to run a view call in a given context.
func RunCall(context view.Context, v func(context view.Context) (interface{}, error)) (interface{}, error) {
	return context.RunView(
		nil,
		view.WithViewCall(v),
	)
}

// Initiate initiates a new protocol whose initiator's view is the passed one.
// The execution happens in a freshly created context.
// This is a shortcut for `view.GetManager(context).InitiateView(initiator)`.
func Initiate(context view.Context, initiator View) (interface{}, error) {
	m, err := GetManager(context)
	if err != nil {
		return nil, err
	}
	return m.InitiateView(initiator, context.Context())
}

// AsResponder can be used by an initiator to behave temporarily as a responder.
// Recall that a responder is characterized by having a default session (`context.Session()`) established by an initiator.
func AsResponder(context view.Context, session view.Session, v func(context view.Context) (interface{}, error)) (interface{}, error) {
	return context.RunView(
		nil,
		view.WithViewCall(v),
		view.AsResponder(session),
	)
}

// AsInitiatorCall can be used by a responder to behave temporarily as an initiator.
// Recall that an initiator is characterized by having an initiator (`context.Initiator()`) set when the initiator is instantiated.
// AsInitiatorCall sets context.Initiator() to the passed initiator, and executes the passed view call.
// TODO: what happens to the sessions already openend with a different initiator (maybe an empty one)?
func AsInitiatorCall(context view.Context, initiator View, v func(context view.Context) (interface{}, error)) (interface{}, error) {
	return context.RunView(
		initiator,
		view.WithViewCall(v),
		view.AsInitiator(),
	)
}

// AsInitiatorView can be used by a responder to behave temporarily as an initiator.
// Recall that an initiator is characterized by having an initiator (`context.Initiator()`) set when the initiator is instantiated.
// AsInitiatorView sets context.Initiator() to the passed initiator, and executes it.
func AsInitiatorView(context view.Context, initiator View) (interface{}, error) {
	return context.RunView(
		initiator,
		view.AsInitiator(),
	)
}

// RunViewNow invokes the Call function of the given view.
// The view context used to the run view is either ctx or the one extracted from the options (see view.WithContext), if present.
func RunViewNow(parent ParentContext, v View, opts ...view.RunViewOption) (res interface{}, err error) {
	options, err := view.CompileRunViewOptions(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed compiling options")
	}
	var initiator View
	if options.AsInitiator {
		initiator = v
	}

	goContext := parent.Context()
	if options.Ctx != nil {
		goContext = options.Ctx
	}

	logger.DebugfContext(goContext, "Start view %s", GetName(v))
	newCtx, span := parent.StartSpanFrom(goContext, GetName(v), tracing.WithAttributes(
		tracing.String(ViewLabel, GetIdentifier(v)),
		tracing.String(InitiatorViewLabel, GetIdentifier(initiator)),
	), trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	var cc ParentContext
	if options.SameContext {
		cc = WrapContext(parent, newCtx)
	} else {
		if options.AsInitiator {
			cc = NewChildContextFromParentAndInitiator(WrapContext(parent, newCtx), initiator)
			// register options.Session under initiator
			contextSession := parent.Session()
			if contextSession == nil {
				return nil, errors.Errorf("cannot convert a non-responder context to an initiator context")
			}
			if err := cc.PutSession(initiator, contextSession.Info().Caller, contextSession); err != nil {
				return nil, errors.Wrapf(err, "failed registering default session as initiated by [%s:%s]", initiator, contextSession.Info().Caller)
			}
		} else {
			cc = NewChildContext(WrapContext(parent, newCtx), options.Session, initiator)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			cc.Cleanup()
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
		cc.Cleanup()
		return nil, err
	}
	return res, err
}

// RunView runs passed view within the passed context and using the passed options in a separate goroutine
func RunView(context view.Context, view View, opts ...view.RunViewOption) {
	defer func() {
		if r := recover(); r != nil {
			logger.Debugf("panic in RunView: %v", r)
		}
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Debugf("panic in RunView: %v", r)
			}
		}()
		_, err := context.RunView(view, opts...)
		if err != nil {
			logger.Errorf("failed to run view: %s", err)
		}
	}()
}
