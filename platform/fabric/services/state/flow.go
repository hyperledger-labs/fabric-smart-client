/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	session2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type receiveView struct {
	unmarshaller Unmarshaller
	state        any
}

func NewReceiveView(state any) *receiveView {
	return &receiveView{state: state, unmarshaller: &JSONCodec{}}
}

func (s receiveView) Call(viewCtx view.Context) (any, error) {
	// Wait to receive a state
	payload, err := session2.ReadMessageWithTimeout(viewCtx.Session(), 30*time.Second)
	if err != nil {
		return nil, err
	}

	err = s.unmarshaller.Unmarshal(payload, s.state)
	if err != nil {
		return nil, errors.Wrap(err, "failed setting state from bytes")
	}

	return s.state, nil
}

type payloadReceiveView struct{}

func NewPayloadReceiveView() *payloadReceiveView {
	return &payloadReceiveView{}
}

func (payloadReceiveView) Call(viewCtx view.Context) (any, error) {
	// Wait to receive a state
	return session2.ReadMessageWithTimeout(viewCtx.Session(), 30*time.Second)
}

type sendReceiveView struct {
	sendState    any
	receiveState any
	coded        Codec
	party        view.Identity
}

func (s *sendReceiveView) Call(viewCtx view.Context) (any, error) {
	session, err := viewCtx.GetSession(viewCtx.Initiator(), s.party)
	if err != nil {
		return nil, err
	}

	// Send a state
	sendStateRaw, err := s.coded.Marshal(s.sendState)
	if err != nil {
		return nil, err
	}
	err = session.Send(viewCtx.Context(), sendStateRaw)
	if err != nil {
		return nil, err
	}

	// Receive another state
	payload, err := session2.ReadMessageWithTimeout(session, 30*time.Second)
	if err != nil {
		return nil, err
	}

	err = s.coded.Unmarshal(payload, s.receiveState)
	if err != nil {
		return nil, err
	}
	return s.receiveState, nil
}

func NewSendReceiveView(sendState, receiveState any, party view.Identity) *sendReceiveView {
	return &sendReceiveView{
		sendState:    sendState,
		receiveState: receiveState,
		party:        party,
		coded:        &JSONCodec{},
	}
}

type replyView struct {
	state      any
	marshaller Marshaller
}

func (s *replyView) Call(viewCtx view.Context) (any, error) {
	session := viewCtx.Session()

	raw, err := s.marshaller.Marshal(s.state)
	if err != nil {
		return nil, err
	}

	err = session.Send(viewCtx.Context(), raw)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func NewReplyView(state any) *replyView {
	return &replyView{state: state, marshaller: &JSONCodec{}}
}
