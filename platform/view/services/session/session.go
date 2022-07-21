/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package session

import (
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func ReadMessageWithTimeout(session Session, d time.Duration) ([]byte, error) {

	timeout := time.NewTimer(d)
	defer timeout.Stop()

	ch := session.Receive()
	var payload []byte
	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.Errorf("received error from remote [%s]", string(msg.Payload))
		}
		payload = msg.Payload
	case <-timeout.C:
		return nil, errors.New("time out reached")
	}

	return payload, nil
}

func ReadFirstMessage(context view.Context) (Session, []byte, error) {
	session := context.Session()
	ch := session.Receive()
	var payload []byte

	timeout := time.NewTimer(time.Second * 30)
	defer timeout.Stop()

	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, nil, errors.Errorf("received error from remote [%s]", string(msg.Payload))
		}
		payload = msg.Payload
	case <-timeout.C:
		return nil, nil, errors.New("time out reached")
	}

	return session, payload, nil
}

func ReadFirstMessageOrPanic(context view.Context) []byte {
	session := context.Session()
	ch := session.Receive()
	var payload []byte

	timeout := time.NewTimer(time.Second * 30)
	defer timeout.Stop()

	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			panic(fmt.Sprintf("received error from remote [%s]", string(msg.Payload)))
		}
		payload = msg.Payload
	case <-timeout.C:
		panic("timeout reached")
	}

	return payload
}
