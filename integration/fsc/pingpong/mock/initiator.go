/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mock

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Params struct {
	Mock bool
}

type Initiator struct {
	*Params
}

func (p *Initiator) Call(viewCtx view.Context) (interface{}, error) {
	// Retrieve responder identity
	identityProvider, err := id.GetProvider(viewCtx)
	assert.NoError(err, "failed getting identity provider")
	responder := identityProvider.Identity("responder")
	var anotherViewCtx view.Context
	if p.Mock {
		c := &DelegatedContext{ViewCtx: viewCtx}
		c.RespondToAs(viewCtx.Initiator(), responder, &Responder{})
		anotherViewCtx = c
	} else {
		anotherViewCtx = viewCtx
	}

	// Open a session to the responder
	session, err := anotherViewCtx.GetSession(anotherViewCtx.Initiator(), responder)
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
