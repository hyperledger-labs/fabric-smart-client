/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package session

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const defaultReceiveTimeout = 10 * time.Second

var logger = logging.MustGetLogger()

type Session = view.Session

type jsonSession struct {
	s   Session
	ctx context.Context
}

func NewJSON(viewCtx view.Context, caller view.View, party view.Identity) (*jsonSession, error) {
	s, err := viewCtx.GetSession(caller, party)
	if err != nil {
		return nil, err
	}
	return NewFromSession(viewCtx, s), nil
}

func NewFromInitiator(viewCtx view.Context, party view.Identity) (*jsonSession, error) {
	s, err := viewCtx.GetSession(viewCtx.Initiator(), party)
	if err != nil {
		return nil, err
	}
	return NewFromSession(viewCtx, s), nil
}

func NewFromSession(viewCtx view.Context, session Session) *jsonSession {
	return newJSONSession(session, viewCtx.Context())
}

func JSON(viewCtx view.Context) *jsonSession {
	return newJSONSession(viewCtx.Session(), viewCtx.Context())
}

func newJSONSession(s Session, ctx context.Context) *jsonSession {
	logger.DebugfContext(ctx, "Open json session to [%s]", logging.Eval(s.Info))
	return &jsonSession{s: s, ctx: ctx}
}

func (j *jsonSession) Receive(state interface{}) error {
	return j.ReceiveWithTimeout(state, defaultReceiveTimeout)
}

func (j *jsonSession) ReceiveWithTimeout(state interface{}, d time.Duration) error {
	raw, err := j.ReceiveRawWithTimeout(d)
	if err != nil {
		return err
	}
	err = json.Unmarshal(raw, state)
	if err != nil {
		return errors.Wrapf(err, "failed unmarshalling state, len [%d]", len(raw))
	}
	return nil
}

func (j *jsonSession) ReceiveRaw() ([]byte, error) {
	return j.ReceiveRawWithTimeout(defaultReceiveTimeout)
}

func (j *jsonSession) ReceiveRawWithTimeout(d time.Duration) ([]byte, error) {
	timeout := time.NewTimer(d)
	defer timeout.Stop()

	// TODO: use opts
	logger.DebugfContext(j.ctx, "Wait to receive")
	ch := j.s.Receive()
	var raw []byte
	select {
	case msg := <-ch:
		if msg == nil {
			logger.ErrorfContext(j.ctx, "Received nil message")
			return nil, errors.New("received message is nil")
		}
		if msg.Status == view.ERROR {
			logger.ErrorfContext(j.ctx, "Received error message")
			return nil, errors.Errorf("received error from remote [%s]", string(msg.Payload))
		}
		raw = msg.Payload
	case <-timeout.C:
		logger.ErrorfContext(j.ctx, "timeout reached")
		return nil, errors.Errorf("time out reached on session [%s]", j.Info().ID)
	case <-j.ctx.Done():
		logger.ErrorfContext(j.ctx, "ctx done: %w", j.ctx.Err())
		return nil, errors.Errorf("ctx done [%s]", j.ctx.Err())
	}
	logger.DebugfContext(j.ctx, "json session, received message [%s]", logging.SHA256Base64(raw))
	return raw, nil
}

func (j *jsonSession) Send(state interface{}) error {
	return j.SendWithContext(j.ctx, state)
}

func (j *jsonSession) SendWithContext(ctx context.Context, state interface{}) error {
	v, err := json.Marshal(state)
	if err != nil {
		return err
	}
	logger.DebugfContext(ctx, "json session, send message [%s]", logging.SHA256Base64(v))
	return j.s.SendWithContext(ctx, v)
}

func (j *jsonSession) SendRaw(ctx context.Context, raw []byte) error {
	logger.DebugfContext(ctx, "json session, send raw message [%s]", logging.SHA256Base64(raw))
	return j.s.SendWithContext(ctx, raw)
}

func (j *jsonSession) SendError(err string) error {
	return j.SendErrorWithContext(j.ctx, err)
}

func (j *jsonSession) SendErrorWithContext(ctx context.Context, err string) error {
	logger.ErrorfContext(ctx, "json session, send error: %w", err)
	return j.s.SendErrorWithContext(ctx, []byte(err))
}

func (j *jsonSession) Session() Session {
	return j.s
}

func (j *jsonSession) Info() view.SessionInfo {
	return j.s.Info()
}
