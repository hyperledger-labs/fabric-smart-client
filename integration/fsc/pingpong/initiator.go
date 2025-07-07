/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type Initiator struct{}

func (p *Initiator) Call(context view.Context) (interface{}, error) {
	// Retrieve responder identity
	identityProvider, err := id.GetProvider(context)
	assert.NoError(err, "failed getting identity provider")
	responder := identityProvider.Identity("responder")

	// Open a session to the responder
	session, err := context.GetSession(context.Initiator(), responder)
	assert.NoError(err) // Send a ping

	err = session.Send([]byte("ping"))
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
