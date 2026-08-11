/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fake

import (
	"context"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Responder answers a single ping with a mock pong.
type Responder struct{}

func (p *Responder) Call(viewCtx view.Context) (any, error) {
	// Retrieve the session opened by the initiator
	session := viewCtx.Session()

	ctx, cancel := context.WithTimeout(viewCtx.Context(), responderTimeout)
	defer cancel()

	// Read the message from the initiator
	ch := session.Receive()
	var payload []byte
	select {
	case msg, ok := <-ch:
		if !ok {
			return nil, errors.Errorf("session [%s] closed while waiting for %s", session.Info().ID, pingMessage)
		}
		payload = msg.Payload
	case <-ctx.Done():
		return nil, errors.Wrap(ctx.Err(), "time out reached")
	}

	// Respond with a mock pong if a ping is received, an error otherwise
	if m := string(payload); m != pingMessage {
		// reply with an error
		sendErr := session.SendError(ctx, fmt.Appendf(nil, "expected %s, got %s", pingMessage, m))
		return nil, errors.Join(errors.Errorf("expected %s, got %s", pingMessage, m), sendErr)
	}

	// reply with a mock pong
	if err := session.Send(ctx, []byte(mockPongMessage)); err != nil {
		return nil, errors.Wrapf(err, "failed to send %s", mockPongMessage)
	}

	// Return
	return "OK", nil
}
