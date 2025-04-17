/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"time"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type Message struct {
	Payload []byte
}

type Initiator struct {
	Message
}

func (p *Initiator) Call(context view.Context) (interface{}, error) {
	// Retrieve responder identity
	responder := view2.GetIdentityProvider(context).Identity("responder")

	// Open a session to the responder
	session, err := context.GetSession(context.Initiator(), responder)
	assert.NoError(err) // Send a ping

	if len(p.Payload) == 0 {
		p.Payload = []byte("ping")
	}

	err = session.Send(p.Payload)
	assert.NoError(err) // Wait for the pong
	ch := session.Receive()
	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		m := string(msg.Payload)
		if m != "pong" {
			return nil, errors.Errorf("expected pong, got %s", m)
		}
	case <-time.After(1 * time.Minute):
		return nil, errors.New("responder didn't pong in time")
	}

	// Return
	return "OK", nil
}
