/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mock

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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

type DelegatedContext struct {
	Ctx        view.Context
	responders []*Responders
}

func (c *DelegatedContext) StartSpanFrom(context.Context, string, ...trace.SpanStartOption) (context.Context, trace.Span) {
	return c.Context(), noop.Span{}
}

func (c *DelegatedContext) StartSpan(string, ...trace.SpanStartOption) trace.Span {
	return noop.Span{}
}

func (c *DelegatedContext) GetService(v interface{}) (interface{}, error) {
	return c.Ctx.GetService(v)
}

func (c *DelegatedContext) ID() string {
	return c.Ctx.ID()
}

func (c *DelegatedContext) RunView(view view.View, opts ...view.RunViewOption) (interface{}, error) {
	return c.Ctx.RunView(view, opts...)
}

func (c *DelegatedContext) Me() view.Identity {
	return c.Ctx.Me()
}

func (c *DelegatedContext) IsMe(id view.Identity) bool {
	return c.Ctx.IsMe(id)
}

func (c *DelegatedContext) Initiator() view.View {
	return c.Ctx.Initiator()
}

func (c *DelegatedContext) GetSessionByID(id string, party view.Identity) (view.Session, error) {
	// TODO: check among the responders
	return c.Ctx.GetSessionByID(id, party)
}

func (c *DelegatedContext) Session() view.Session {
	return c.Ctx.Session()
}

func (c *DelegatedContext) Context() context.Context {
	return c.Ctx.Context()
}

func (c *DelegatedContext) OnError(callback func()) {
	c.Ctx.OnError(callback)
}

func (c *DelegatedContext) GetSession(caller view.View, party view.Identity, boundToViews ...view.View) (view.Session, error) {
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
			view2.RunView(c, responder.ResponderView, view.AsResponder(right))

			responder.Channel = biChannel
			return left, nil
		}
	}

	return c.Ctx.GetSession(caller, party)
}

func (c *DelegatedContext) RespondToAs(initiator view.View, responder view.Identity, r view.View) {
	c.responders = append(c.responders, &Responders{
		InitiatorView: initiator,
		ResponderID:   responder,
		ResponderView: r,
	})
}
