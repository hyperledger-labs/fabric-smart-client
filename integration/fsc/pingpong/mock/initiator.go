/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mock

import (
	"time"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	view3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type Params struct {
	Mock bool
}

type Initiator struct {
	*Params
}

func (p *Initiator) Call(ctx view.Context) (interface{}, error) {
	// Retrieve responder identity
	responder := view2.GetIdentityProvider(ctx).Identity("responder")
	var context view.Context
	if p.Mock {
		c := &view3.MockContext{Ctx: ctx}
		c.RespondToAs(ctx.Initiator(), responder, &Responder{})
		context = c
	} else {
		context = ctx
	}

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
		if m != "mock pong" {
			return nil, errors.Errorf("expected mock pong, got %s", m)
		}
	case <-time.After(1 * time.Minute):
		return nil, errors.New("responder didn't pong in time")
	}

	// Return
	return "OK", nil
}
