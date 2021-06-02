/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package state

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type receiveView struct {
	unmarshaller Unmarshaller
	state        interface{}
}

func NewReceiveView(state interface{}) *receiveView {
	return &receiveView{state: state, unmarshaller: &JSONCodec{}}
}

func (s receiveView) Call(context view.Context) (interface{}, error) {
	session := context.Session()

	// Wait to receive a state
	ch := session.Receive()

	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}

		err := s.unmarshaller.Unmarshal(msg.Payload, s.state)
		if err != nil {
			return nil, errors.Wrap(err, "failed setting state from bytes")
		}

		return s.state, nil
	case <-time.After(30 * time.Second):
		return nil, errors.New("timeout reading from session")
	}

}

type payloadReceiveView struct {
}

func NewPayloadReceiveView() *payloadReceiveView {
	return &payloadReceiveView{}
}

func (s payloadReceiveView) Call(context view.Context) (interface{}, error) {
	// Wait to receive a state
	ch := context.Session().Receive()

	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		return msg.Payload, nil
	case <-time.After(30 * time.Second):
		return nil, errors.New("timeout reading from session")
	}
}

type sendReceiveView struct {
	sendState    interface{}
	receiveState interface{}
	coded        Codec
	party        view.Identity
}

func (s *sendReceiveView) Call(context view.Context) (interface{}, error) {
	session, err := context.GetSession(context.Initiator(), s.party)
	if err != nil {
		return nil, err
	}
	// Wait to receive a content back
	ch := session.Receive()

	// Send a state
	sendStateRaw, err := s.coded.Marshal(s.sendState)
	if err != nil {
		return nil, err
	}
	err = session.Send(sendStateRaw)
	if err != nil {
		return nil, err
	}

	// Receive another state
	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}

		err = s.coded.Unmarshal(msg.Payload, s.receiveState)
		if err != nil {
			return nil, err
		}
		return s.receiveState, nil
	case <-time.After(30 * time.Second):
		return nil, errors.New("timeout reading from session")
	}
}

func NewSendReceiveView(sendState, receiveState interface{}, party view.Identity) *sendReceiveView {
	return &sendReceiveView{
		sendState:    sendState,
		receiveState: receiveState,
		party:        party,
		coded:        &JSONCodec{},
	}
}

type replyView struct {
	state      interface{}
	marshaller Marshaller
}

func (s *replyView) Call(context view.Context) (interface{}, error) {
	session := context.Session()

	raw, err := s.marshaller.Marshal(s.state)
	if err != nil {
		return nil, err
	}

	err = session.Send(raw)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func NewReplyView(state interface{}) *replyView {
	return &replyView{state: state, marshaller: &JSONCodec{}}
}
