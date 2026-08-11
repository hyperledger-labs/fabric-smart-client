/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

const (
	pingMessage     = "ping"
	pongMessage     = "pong"
	finishedMessage = "finished"

	// DefaultRounds is the number of ping/pong rounds run when Params.Rounds is not set.
	DefaultRounds = 3

	// roundTimeout bounds a single round end to end: session lookup (which may have to open a
	// new stream), send, and wait for the peer's answer. Initiator and responder use the same
	// value, so that neither gives up on the other prematurely.
	roundTimeout = 1 * time.Minute
)

// Params is the input of the Initiator view.
type Params struct {
	// Rounds is the number of ping/pong rounds to run; values <= 0 select DefaultRounds.
	Rounds int `json:"rounds,omitempty"`
}

// PingView sends a single ping and waits for the pong over the context's session.
type PingView struct {
	responder view.Identity
}

func (p *PingView) Call(viewCtx view.Context) (any, error) {
	session, err := viewCtx.GetSession(viewCtx.Initiator(), p.responder)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting session to [%s]", p.responder)
	}

	logger.DebugfContext(viewCtx.Context(), "send %s", pingMessage)
	if err := session.Send(viewCtx.Context(), []byte(pingMessage)); err != nil {
		return nil, errors.Wrapf(err, "failed to send %s", pingMessage)
	}

	ch := session.Receive()
	select {
	case msg, ok := <-ch:
		if !ok {
			return nil, errors.Errorf("session [%s] closed while waiting for %s", session.Info().ID, pongMessage)
		}
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		if m := string(msg.Payload); m != pongMessage {
			return nil, errors.Errorf("expected %s, got %s", pongMessage, m)
		}
		logger.DebugfContext(viewCtx.Context(), "%s received", pongMessage)
	case <-viewCtx.Context().Done():
		return nil, errors.Wrapf(viewCtx.Context().Err(), "no %s received in time", pongMessage)
	}

	return nil, nil
}

// FinishedView tells the responder that the initiator is done. Closing a session is a local
// operation only and puts nothing on the wire, so without this message the responder would
// keep waiting for the next ping.
type FinishedView struct {
	responder view.Identity
}

func (f *FinishedView) Call(viewCtx view.Context) (any, error) {
	session, err := viewCtx.GetSession(viewCtx.Initiator(), f.responder)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting session to [%s]", f.responder)
	}

	logger.DebugfContext(viewCtx.Context(), "send %s", finishedMessage)
	if err := session.SendWithContext(viewCtx.Context(), []byte(finishedMessage)); err != nil {
		return nil, errors.Wrapf(err, "failed to send %s", finishedMessage)
	}
	return nil, nil
}

// runRound runs v in a context of its own, bounded by roundTimeout and cancelled as soon as the
// round is over. Every round gets a fresh, short-lived context so that a stale round's
// cancellation can never affect a later one that happens to reuse the same cached stream; see
// the regression tests in platform/view/services/comm/host/websocket/ws/context_detachment_test.go.
func runRound(viewCtx view.Context, v view.View) (any, error) {
	ctx, cancel := context.WithTimeout(viewCtx.Context(), roundTimeout)
	defer cancel()

	return view2.WrapContext(viewCtx, ctx).RunView(v)
}

func ping(viewCtx view.Context, responder view.Identity) (any, error) {
	return runRound(viewCtx, &PingView{responder: responder})
}

func finish(viewCtx view.Context, responder view.Identity) (any, error) {
	return runRound(viewCtx, &FinishedView{responder: responder})
}

// Initiator runs Rounds (or DefaultRounds, if Rounds is unset) ping/pong rounds against the
// responder identity, then tells the responder the protocol is over.
type Initiator struct {
	Params
}

func (p *Initiator) Call(viewCtx view.Context) (any, error) {
	identityProvider, err := id.GetProvider(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting identity provider")
	}
	responder := identityProvider.Identity("responder")

	rounds := p.rounds()
	logger.DebugfContext(viewCtx.Context(), "ping pong with [%s] over [%d] rounds", responder, rounds)
	for round := range rounds {
		if _, err := ping(viewCtx, responder); err != nil {
			return nil, errors.Wrapf(err, "ping round [%d/%d] failed", round+1, rounds)
		}
	}
	if _, err := finish(viewCtx, responder); err != nil {
		return nil, errors.Wrapf(err, "failed signalling the end of the protocol after [%d] rounds", rounds)
	}

	return "OK", nil
}

func (p *Initiator) rounds() int {
	if p.Rounds <= 0 {
		return DefaultRounds
	}
	return p.Rounds
}
