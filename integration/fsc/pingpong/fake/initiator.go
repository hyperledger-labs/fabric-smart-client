/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fake

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	pingMessage     = "ping"
	mockPongMessage = "mock pong"

	// initiatorTimeout bounds how long the initiator waits for the mock pong.
	initiatorTimeout = 1 * time.Minute
	// responderTimeout bounds how long the responder waits for the ping.
	responderTimeout = 5 * time.Second
)

// Params is the input of the Initiator view.
type Params struct {
	Mock bool
}

// Initiator sends a single ping and waits for a pong. When Mock is set, the pong is produced
// locally by a Responder run through a DelegatedContext instead of a real remote session.
type Initiator struct {
	*Params
}

func (p *Initiator) Call(viewCtx view.Context) (any, error) {
	// Retrieve responder identity
	identityProvider, err := id.GetProvider(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting identity provider")
	}
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
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting session to [%s]", responder)
	}

	ctx, cancel := context.WithTimeout(viewCtx.Context(), initiatorTimeout)
	defer cancel()

	// Send a ping
	if err := session.SendWithContext(ctx, []byte(pingMessage)); err != nil {
		return nil, errors.Wrapf(err, "failed to send %s", pingMessage)
	}

	// Wait for the pong
	ch := session.Receive()
	select {
	case msg, ok := <-ch:
		if !ok {
			return nil, errors.Errorf("session [%s] closed while waiting for %s", session.Info().ID, mockPongMessage)
		}
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		if m := string(msg.Payload); m != mockPongMessage {
			return nil, errors.Errorf("expected %s, got %s", mockPongMessage, m)
		}
	case <-ctx.Done():
		return nil, errors.Wrapf(ctx.Err(), "no %s received in time", mockPongMessage)
	}

	// Return
	return "OK", nil
}
