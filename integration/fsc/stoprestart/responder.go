/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Responder struct{}

func (p *Responder) Call(viewCtx view.Context) (interface{}, error) {
	// Retrieve the session opened by the initiator
	session := viewCtx.Session()

	// Read the message from the initiator
	ch := session.Receive()
	var payload []byte
	var rcvCtx context.Context
	var rcvSpan trace.Span
	select {
	case msg := <-ch:
		payload = msg.Payload
		rcvCtx, rcvSpan = viewCtx.StartSpanFrom(msg.Ctx, "responder_receive", trace.WithSpanKind(trace.SpanKindServer))
		defer rcvSpan.End()
		rcvSpan.AddLink(trace.Link{SpanContext: trace.SpanContextFromContext(viewCtx.Context())})
	case <-time.After(5 * time.Second):
		return nil, errors.New("time out reached")
	}

	// Respond with a pong if a ping is received, an error otherwise
	m := string(payload)
	switch {
	case m != "ping":
		// reply with an error
		err := session.SendErrorWithContext(rcvCtx, []byte(fmt.Sprintf("exptectd ping, got %s", m)))
		assert.NoError(err)
		return nil, errors.Errorf("exptectd ping, got %s", m)
	default:
		// reply with pong
		err := session.SendWithContext(rcvCtx, []byte("pong"))
		assert.NoError(err)
	}

	// Return
	return "OK", nil
}
