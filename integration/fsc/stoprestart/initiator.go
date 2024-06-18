/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

import (
	"time"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type Initiator struct{}

func (p *Initiator) Call(context view.Context) (interface{}, error) {
	span := context.StartSpan("call_initiator_view")
	defer span.End()
	// Retrieve responder identity
	responder := view2.GetIdentityProvider(context).Identity("bob")
	responder2 := view2.GetIdentityProvider(context).Identity("bob_alias")
	assert.Equal(responder, responder2, "expected same identity for bob and its alias")

	// Open a session to the responder
	span.AddEvent("open_session")
	session, err := context.GetSession(context.Initiator(), responder)
	assert.NoError(err) // Send a ping
	span.AddEvent("send_ping")
	err = session.SendWithContext(context.Context(), []byte("ping"))
	assert.NoError(err) // Wait for the pong
	span.AddEvent("wait_pong")
	ch := session.Receive()
	span.AddEvent("received_response")
	select {
	case msg := <-ch:
		_, rcvSpan := context.StartSpanFrom(msg.Ctx, "initiator_receive")
		defer rcvSpan.End()
		rcvSpan.AddLink(trace.Link{SpanContext: span.SpanContext()})
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		span.AddEvent("read_response")
		rcvSpan.AddEvent("read_response")
		m := string(msg.Payload)
		if m != "pong" {
			return nil, errors.Errorf("exptectd pong, got %s", m)
		}
	case <-time.After(1 * time.Minute):
		return nil, errors.New("responder didn't pong in time")
	}

	// Return
	return "OK", nil
}
