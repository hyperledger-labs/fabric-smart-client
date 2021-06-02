/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package session

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("view-sdk.session.json")

type Session interface {
	view.Session
}

type jsonSession struct {
	s       Session
	context context.Context
}

func NewJSon(context view.Context, caller view.View, party view.Identity) (*jsonSession, error) {
	s, err := context.GetSession(caller, party)
	if err != nil {
		return nil, err
	}
	return &jsonSession{s: s, context: context.Context()}, nil
}

func JSon(context view.Context) *jsonSession {
	return &jsonSession{s: context.Session(), context: context.Context()}
}

func (j *jsonSession) Receive(state interface{}) error {
	ch := j.s.Receive()
	var raw []byte
	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return errors.Errorf("received error from remote [%s]", string(msg.Payload))
		}
		raw = msg.Payload
	case <-time.After(10 * time.Second):
		return errors.New("time out reached")
	case <-j.context.Done():
		return errors.Errorf("context done [%s]", j.context.Err())
	}
	logger.Debugf("json session, received message [%s]", hash.Hashable(raw).String())
	return json.Unmarshal(raw, state)
}

func (j *jsonSession) ReceiveWithTimeout(state interface{}, d time.Duration) error {
	// TODO: use opts
	ch := j.s.Receive()
	var raw []byte
	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return errors.Errorf("received error from remote [%s]", string(msg.Payload))
		}
		raw = msg.Payload
	case <-time.After(d):
		return errors.New("time out reached")
	case <-j.context.Done():
		return errors.Errorf("context done [%s]", j.context.Err())
	}
	logger.Debugf("json session, received message [%s]", hash.Hashable(raw).String())
	return json.Unmarshal(raw, state)
}

func (j *jsonSession) Send(state interface{}) error {
	v, err := json.Marshal(state)
	if err != nil {
		return err
	}
	logger.Debugf("json session, send message [%s]", hash.Hashable(v).String())
	return j.s.Send(v)
}

func (j *jsonSession) SendError(err string) error {
	logger.Debugf("json session, send error [%s]", err)
	return j.s.SendError([]byte(err))
}

func (j *jsonSession) Session() Session {
	return j.s
}
