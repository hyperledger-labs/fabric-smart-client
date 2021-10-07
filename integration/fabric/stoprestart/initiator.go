/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Initiator struct {
	in []byte
}

func (p *Initiator) Call(context view.Context) (interface{}, error) {
	// Retrieve responder identity
	responder := view2.GetIdentityProvider(context).Identity("bob")

	// Open a session to the responder
	session, err := context.GetSession(context.Initiator(), responder)
	assert.NoError(err)
	// Send your input from the client
	err = session.Send(p.in)
	assert.NoError(err)
	// Wait to receive it back
	ch := session.Receive()
	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		if bytes.Equal(msg.Payload, p.in) {
			return nil, fmt.Errorf("exptectd %s, got %s", string(p.in), string(msg.Payload))
		}
	case <-time.After(1 * time.Minute):
		return nil, errors.New("responder didn't pong in time")
	}

	// Return
	return "OK", nil
}

type InitiatorViewFactory struct{}

func (i *InitiatorViewFactory) NewView(in []byte) (view.View, error) {
	return &Initiator{in: in}, nil
}
