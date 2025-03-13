/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

const defaultReceiveTimeout = 10 * time.Second

var logger = logging.MustGetLogger("view-sdk.session.json")

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
	span := trace.SpanFromContext(context)
	span.AddEvent(fmt.Sprintf("Open json session to [%s:%s]", string(s.Info().Caller), s.Info().CallerViewID))
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
	return json.Unmarshal(raw, state)
}

func (j *jsonSession) ReceiveRaw() ([]byte, error) {
	return j.ReceiveRawWithTimeout(defaultReceiveTimeout)
}

func (j *jsonSession) ReceiveRawWithTimeout(d time.Duration) ([]byte, error) {
	span := trace.SpanFromContext(j.context)
	timeout := time.NewTimer(d)
	defer timeout.Stop()

	// TODO: use opts
	span.AddEvent("Wait to receive")
	ch := j.s.Receive()
	var raw []byte
	select {
	case msg := <-ch:
		span.AddEvent("Received message")
		if msg.Status == view.ERROR {
			return nil, errors.Errorf("received error from remote [%s]", string(msg.Payload))
		}
		raw = msg.Payload
	case <-timeout.C:
		err := errors.New("time out reached")
		span.RecordError(err)
		return nil, err
	case <-j.context.Done():
		err := errors.Errorf("context done [%s]", j.context.Err())
		span.RecordError(err)
		return nil, err
	}
	logger.Debugf("json session, received message [%s]", hash.Hashable(raw))
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
	logger.Debugf("json session, send message [%s]", hash.Hashable(v).String())
	return j.s.SendWithContext(ctx, v)
}

func (j *jsonSession) SendRaw(ctx context.Context, raw []byte) error {
	logger.Debugf("json session, send raw message [%s]", hash.Hashable(raw).String())
	return j.s.SendWithContext(ctx, raw)
}

func (j *jsonSession) SendError(err string) error {
	return j.SendErrorWithContext(j.context, err)
}

func (j *jsonSession) SendErrorWithContext(ctx context.Context, err string) error {
	span := trace.SpanFromContext(ctx)
	span.RecordError(errors.New(err))
	logger.Debugf("json session, send error [%s]", err)
	return j.s.SendErrorWithContext(ctx, []byte(err))
}

func (j *jsonSession) Session() Session {
	return j.s
}

func (j *jsonSession) Info() view.SessionInfo {
	return j.s.Info()
}
