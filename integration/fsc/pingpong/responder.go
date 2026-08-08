/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"context"
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// PongView waits for a single ping and answers with a pong over the context's session.
type PongView struct {
	session view.Session
}

func (p *PongView) Call(viewCtx view.Context) (any, error) {
	session := p.session

	// Read the message from the initiator
	ch := session.Receive()
	var payload []byte
	select {
	case msg := <-ch:
		payload = msg.Payload
	case <-viewCtx.Context().Done():
		return nil, viewCtx.Context().Err()
	case <-time.After(5 * time.Second):
		return nil, errors.New("time out reached")
	}

	// Respond with a pong if a ping is received, an error otherwise
	m := string(payload)
	if m != "ping" {
		// reply with an error
		err := session.SendErrorWithContext(viewCtx.Context(), fmt.Appendf(nil, "expected ping, got %s", m))
		assert.NoError(err)
		return nil, errors.Errorf("expected ping, got %s", m)
	}

	logger.Infof("ping received, send pong...")
	// reply with pong
	err := session.SendWithContext(viewCtx.Context(), []byte("pong"))
	assert.NoError(err)

	return nil, nil
}

func pong(viewCtx view.Context, session view.Session) (any, error) {
	ctx, cancel := context.WithTimeout(viewCtx.Context(), 10*time.Minute)
	runCtx := view2.WrapContext(viewCtx, ctx)
	defer func() {
		logger.Infof("call cancel on view context [%s:%s]", runCtx.ID(), viewCtx.ID())
		cancel()
	}()

	return runCtx.RunView(&PongView{
		session: session,
	})
}

type Responder struct{}

func (p *Responder) Call(viewCtx view.Context) (any, error) {
	// Retrieve the session opened by the initiator
	session := viewCtx.Session()

	for range 3 {
		if _, err := pong(viewCtx, session); err != nil {
			return nil, err
		}
	}

	// Return
	return "OK", nil
}
