/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"errors"
	"fmt"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Responder struct{}

func (p *Responder) Call(context view.Context) (interface{}, error) {
	// Retrieve the session opened by the initiator
	session := context.Session()

	// Read the message from the initiator
	ch := session.Receive()
	var payload []byte
	select {
	case msg := <-ch:
		payload = msg.Payload
	case <-time.After(5 * time.Second):
		return nil, errors.New("time out reached")
	}

	// Respond with a pong if a ping is received, an error otherwise
	m := string(payload)
	switch {
	case m != "ping":
		// reply with an error
		err := session.SendError([]byte(fmt.Sprintf("exptectd ping, got %s", m)))
		assert.NoError(err)
		return nil, fmt.Errorf("exptectd ping, got %s", m)
	default:
		tracing.Get(context).EmitKey(0, "received", "ping")
		// reply with pong
		err := session.Send([]byte("pong"))
		assert.NoError(err)
		tracing.Get(context).EmitKey(0, "sent", "pong")
	}

	// Return
	return "OK", nil
}
