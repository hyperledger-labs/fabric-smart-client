/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type Responders struct {
	InitiatorView view.View
	ResponderID   view.Identity
	ResponderView view.View
	Lock          sync.RWMutex
	Channel       *session.LocalBidirectionalChannel
}

type MockContext struct {
	Ctx        view.Context
	responders []*Responders
}

func (c *MockContext) StartSpanFrom(context.Context, string, ...trace.SpanStartOption) (context.Context, trace.Span) {
	return c.Context(), noop.Span{}
}

func (c *MockContext) StartSpan(string, ...trace.SpanStartOption) trace.Span {
	return noop.Span{}
}

func (c *MockContext) GetService(v any) (any, error) {
	return c.Ctx.GetService(v)
}

func (c *MockContext) ID() string {
	return c.Ctx.ID()
}

func (c *MockContext) RunView(view view.View, opts ...view.RunViewOption) (any, error) {
	return c.Ctx.RunView(view, opts...)
}

func (c *MockContext) Me() view.Identity {
	return c.Ctx.Me()
}

func (c *MockContext) IsMe(id view.Identity) bool {
	return c.Ctx.IsMe(id)
}

func (c *MockContext) Initiator() view.View {
	return c.Ctx.Initiator()
}

func (c *MockContext) GetSessionByID(id string, party view.Identity) (view.Session, error) {
	// TODO: check among the responders
	return c.Ctx.GetSessionByID(id, party)
}

func (c *MockContext) Session() view.Session {
	return c.Ctx.Session()
}

func (c *MockContext) Context() context.Context {
	return c.Ctx.Context()
}

func (c *MockContext) OnError(callback func()) {
	c.Ctx.OnError(callback)
}

func (c *MockContext) GetSession(caller view.View, party view.Identity, boundToViews ...view.View) (view.Session, error) {
	for _, responder := range c.responders {
		if responder.InitiatorView == caller && responder.ResponderID.Equal(party) {
			responder.Lock.RLock()
			if responder.Channel != nil {
				responder.Lock.Unlock()
				return responder.Channel.LeftSession(), nil
			}
			responder.Lock.RUnlock()

			responder.Lock.Lock()
			defer responder.Lock.Unlock()

			if responder.Channel != nil {
				return responder.Channel.LeftSession(), nil
			}

			// instantiate the mock view
			biChannel, err := session.NewLocalBidirectionalChannel("", c.ID(), "", nil)
			if err != nil {
				return nil, errors.Wrap(err, "failed creating session")
			}
			left := biChannel.LeftSession()
			right := biChannel.RightSession()
			RunView(c, responder.ResponderView, view.AsResponder(right))

			responder.Channel = biChannel
			return left, nil
		}
	}

	return c.Ctx.GetSession(caller, party)
}

func (c *MockContext) RespondToAs(initiator view.View, responder view.Identity, r view.View) {
	c.responders = append(c.responders, &Responders{
		InitiatorView: initiator,
		ResponderID:   responder,
		ResponderView: r,
	})
}

// RunView runs passed view within the passed context and using the passed options in a separate goroutine
func RunView(context view.Context, view view.View, opts ...view.RunViewOption) {
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
