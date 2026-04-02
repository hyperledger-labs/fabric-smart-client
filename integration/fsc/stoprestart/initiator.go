/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.opentelemetry.io/otel/trace"
)

var logger = logging.MustGetLogger()

type Initiator struct{}

func (p *Initiator) Call(viewCtx view.Context) (interface{}, error) {
	// Retrieve responder identity
	identityProvider, err := id.GetProvider(viewCtx)
	assert.NoError(err, "failed getting identity provider")
	responder := identityProvider.Identity("bob")
	responder2 := identityProvider.Identity("bob_alias")
	assert.Equal(responder, responder2, "expected same identity for bob and its alias")

	// Open a session to the responder
	logger.DebugfContext(viewCtx.Context(), "open_session")
	session, err := viewCtx.GetSession(viewCtx.Initiator(), responder)
	assert.NoError(err) // Send a ping
	logger.DebugfContext(viewCtx.Context(), "send_ping")
	err = session.SendWithContext(viewCtx.Context(), []byte("ping"))
	assert.NoError(err) // Wait for the pong
	logger.DebugfContext(viewCtx.Context(), "wait_pong")
	ch := session.Receive()
	logger.DebugfContext(viewCtx.Context(), "received_response")
	select {
	case msg := <-ch:
		_, rcvSpan := viewCtx.StartSpanFrom(msg.Ctx, "initiator_receive")
		defer rcvSpan.End()
		rcvSpan.AddLink(trace.Link{SpanContext: trace.SpanContextFromContext(viewCtx.Context())})
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		logger.DebugfContext(viewCtx.Context(), "read_response")
		rcvSpan.AddEvent("Read response")
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
