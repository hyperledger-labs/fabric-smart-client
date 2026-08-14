/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// PongView waits for a single message and, depending on what it is, either answers with a pong
// (and reports it as having handled a ping) or reports that the protocol is over. It returns the
// message it handled, leaving the decision on whether to keep going to the caller.
type PongView struct {
	session view.Session
}

func (p *PongView) Call(viewCtx view.Context) (any, error) {
	ch := p.session.Receive()

	var payload []byte
	select {
	case msg, ok := <-ch:
		if !ok {
			return nil, errors.Errorf("session [%s] closed while waiting for the next message", p.session.Info().ID)
		}
		if msg.Status == view.ERROR {
			return nil, errors.Errorf("initiator failed: %s", string(msg.Payload))
		}
		payload = msg.Payload
	case <-viewCtx.Context().Done():
		return nil, errors.Wrap(viewCtx.Context().Err(), "no message received in time")
	}

	switch m := string(payload); m {
	case pingMessage:
		logger.DebugfContext(viewCtx.Context(), "%s received, send %s", pingMessage, pongMessage)
		if err := p.session.SendWithContext(viewCtx.Context(), []byte(pongMessage)); err != nil {
			return nil, errors.Wrapf(err, "failed to send %s", pongMessage)
		}
		return m, nil
	case finishedMessage:
		logger.DebugfContext(viewCtx.Context(), "%s received, nothing to answer", finishedMessage)
		return m, nil
	default:
		sendErr := p.session.SendErrorWithContext(viewCtx.Context(), fmt.Appendf(nil, "expected %s or %s, got %s", pingMessage, finishedMessage, m))
		return nil, errors.Join(errors.Errorf("expected %s or %s, got %s", pingMessage, finishedMessage, m), sendErr)
	}
}

func pong(viewCtx view.Context, session view.Session) (string, error) {
	res, err := runRound(viewCtx, &PongView{session: session})
	if err != nil {
		return "", err
	}
	m, ok := res.(string)
	if !ok {
		return "", errors.Errorf("unexpected result [%v] of type [%T] from the pong view", res, res)
	}
	return m, nil
}

// Responder answers pings with pongs until the initiator signals that the protocol is over.
type Responder struct{}

func (p *Responder) Call(viewCtx view.Context) (any, error) {
	session := viewCtx.Session()
	if session == nil {
		return nil, errors.New("no default session, the responder must be invoked by an initiator")
	}

	for round := 0; ; round++ {
		m, err := pong(viewCtx, session)
		if err != nil {
			return nil, errors.Wrapf(err, "pong round [%d] failed", round+1)
		}
		if m == finishedMessage {
			logger.DebugfContext(viewCtx.Context(), "initiator finished after [%d] rounds", round)
			return "OK", nil
		}
	}
}
