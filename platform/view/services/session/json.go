/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package session

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

const defaultReceiveTimeout = 10 * time.Second

var logger = logging.MustGetLogger("view-sdk.session.json")

type Session interface {
	view.Session
}

type jsonSession struct {
	s       Session
	context context.Context
}

func NewJSON(context view.Context, caller view.View, party view.Identity) (*jsonSession, error) {
	s, err := context.GetSession(caller, party)
	if err != nil {
		return nil, err
	}
	return &jsonSession{s: s, context: context.Context()}, nil
}

func JSON(context view.Context) *jsonSession {
	return &jsonSession{s: context.Session(), context: context.Context()}
}

func (j *jsonSession) Receive(state interface{}) error {
	return j.ReceiveWithTimeout(state, defaultReceiveTimeout)
}

func (j *jsonSession) ReceiveWithTimeout(state interface{}, d time.Duration) error {
	span := trace.SpanFromContext(j.context)
	timeout := time.NewTimer(d)
	defer timeout.Stop()

	// TODO: use opts
	span.AddEvent("wait_receive")
	ch := j.s.Receive()
	var raw []byte
	select {
	case msg := <-ch:
		span.AddEvent("received_message")
		if msg.Status == view.ERROR {
			return errors.Errorf("received error from remote [%s]", string(msg.Payload))
		}
		raw = msg.Payload
	case <-timeout.C:
		span.AddEvent("timeout")
		return errors.New("time out reached")
	case <-j.context.Done():
		return errors.Errorf("context done [%s]", j.context.Err())
	}
	logger.Debugf("json session, received message [%s]", hash.Hashable(raw))
	span.AddEvent("unmarshal_response")
	return json.Unmarshal(raw, state)
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
	logger.Debugf("json session, send error [%s]", err)
	return j.s.SendError([]byte(err))
}

func (j *jsonSession) Session() Session {
	return j.s
}
