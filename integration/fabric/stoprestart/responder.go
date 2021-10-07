/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

import (
	"errors"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
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

	// Echo back what you receved from the initiator
	err := session.Send(payload)
	assert.NoError(err)

	// Return
	return "OK", nil
}
