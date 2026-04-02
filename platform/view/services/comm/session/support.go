/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package session

import (
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func ReadMessageWithTimeout(session Session, d time.Duration) ([]byte, error) {
	timeout := time.NewTimer(d)
	defer timeout.Stop()

	ch := session.Receive()
	if ch == nil {
		return nil, errors.New("session receive channel is nil")
	}
	var payload []byte
	select {
	case msg, ok := <-ch:
		if !ok {
			return nil, errors.New("session receive channel is closed")
		}
		if msg == nil {
			return nil, errors.New("received message is nil")
		}
		if msg.Status == view.ERROR {
			return nil, errors.Errorf("received error from remote [%s]", string(msg.Payload))
		}
		payload = msg.Payload
	case <-timeout.C:
		return nil, errors.New("time out reached")
	}

	return payload, nil
}

func ReadFirstMessage(viewCtx view.Context) (Session, []byte, error) {
	session := viewCtx.Session()
	ch := session.Receive()
	if ch == nil {
		return nil, nil, errors.New("session receive channel is nil")
	}
	var payload []byte

	timeout := time.NewTimer(time.Second * 30)
	defer timeout.Stop()

	select {
	case msg, ok := <-ch:
		if !ok {
			return nil, nil, errors.New("session receive channel is closed")
		}
		if msg == nil {
			return nil, nil, errors.New("received message is nil")
		}
		if msg.Status == view.ERROR {
			return nil, nil, errors.Errorf("received error from remote [%s]", string(msg.Payload))
		}
		payload = msg.Payload
	case <-timeout.C:
		return nil, nil, errors.New("time out reached")
	}

	return session, payload, nil
}

func ReadFirstMessageOrPanic(viewCtx view.Context) []byte {
	session := viewCtx.Session()
	ch := session.Receive()
	if ch == nil {
		panic("session receive channel is nil")
	}
	var payload []byte

	timeout := time.NewTimer(time.Second * 30)
	defer timeout.Stop()

	select {
	case msg, ok := <-ch:
		if !ok {
			panic("session receive channel is closed")
		}
		if msg == nil {
			panic("received message is nil")
		}
		if msg.Status == view.ERROR {
			panic(fmt.Sprintf("received error from remote [%s]", string(msg.Payload)))
		}
		payload = msg.Payload
	case <-timeout.C:
		panic("timeout reached")
	}

	return payload
}
