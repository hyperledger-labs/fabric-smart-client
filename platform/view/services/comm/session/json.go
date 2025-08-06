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
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const defaultReceiveTimeout = 10 * time.Second

var logger = logging.MustGetLogger()

type Session = view.Session

type jsonSession struct {
	s       Session
	context context.Context
}

func NewJSON(context view.Context, caller view.View, party view.Identity) (*jsonSession, error) {
	s, err := context.GetSession(caller, party)
	if err != nil {
		return nil, err
	}
	return NewFromSession(context, s), nil
}

func NewFromInitiator(context view.Context, party view.Identity) (*jsonSession, error) {
	s, err := context.GetSession(context.Initiator(), party)
	if err != nil {
		return nil, err
	}
	return NewFromSession(context, s), nil
}

func NewFromSession(context view.Context, session Session) *jsonSession {
	return newJSONSession(session, context.Context())
}

func JSON(context view.Context) *jsonSession {
	return newJSONSession(context.Session(), context.Context())
}

func newJSONSession(s Session, context context.Context) *jsonSession {
	logger.DebugfContext(context, "Open json session to [%s]", logging.Eval(s.Info))
	return &jsonSession{s: s, context: context}
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
	logger.DebugfContext(j.context, "Wait to receive")
	ch := j.s.Receive()
	var raw []byte
	select {
	case msg := <-ch:
		if msg == nil {
			logger.ErrorfContext(j.context, "Received nil message")
			return nil, errors.New("received message is nil")
		}
		if msg.Status == view.ERROR {
			logger.ErrorfContext(j.context, "Received error message")
			return nil, errors.Errorf("received error from remote [%s]", string(msg.Payload))
		}
		raw = msg.Payload
	case <-timeout.C:
		logger.ErrorfContext(j.context, "timeout reached")
		return nil, errors.New("time out reached")
	case <-j.context.Done():
		logger.ErrorfContext(j.context, "context done: %w", j.context.Err())
		return nil, errors.Errorf("context done [%s]", j.context.Err())
	}
	logger.DebugfContext(j.context, "json session, received message [%s]", hash.Hashable(raw))
	return raw, nil
}

func (j *jsonSession) Send(state interface{}) error {
	return j.SendWithContext(j.context, state)
}

func (j *jsonSession) SendWithContext(ctx context.Context, state interface{}) error {
	v, err := json.Marshal(state)
	if err != nil {
		return err
	}
	logger.DebugfContext(ctx, "json session, send message [%s]", hash.Hashable(v).String())
	return j.s.SendWithContext(ctx, v)
}

func (j *jsonSession) SendRaw(ctx context.Context, raw []byte) error {
	logger.DebugfContext(ctx, "json session, send raw message [%s]", hash.Hashable(raw).String())
	return j.s.SendWithContext(ctx, raw)
}

func (j *jsonSession) SendError(err string) error {
	return j.SendErrorWithContext(j.context, err)
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
