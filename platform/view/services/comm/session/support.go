/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package session

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ReadMessageWithTimeout returns the payload of the next message on session, waiting at most d.
// An error reply from the remote end is returned as RejectionError's error.
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
			return nil, RejectionError(msg)
		}
		payload = msg.Payload
	case <-timeout.C:
		return nil, errors.New("time out reached")
	}

	return payload, nil
}

// ReadFirstMessage returns viewCtx's session and the payload of its next message, waiting at
// most 30 seconds. An error reply from the remote end is returned as RejectionError's error.
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
			return nil, nil, RejectionError(msg)
		}
		payload = msg.Payload
	case <-timeout.C:
		return nil, nil, errors.New("time out reached")
	}

	return session, payload, nil
}

// ReadFirstMessageOrPanic is ReadFirstMessage that returns only the payload and panics instead
// of returning an error; for an error reply it panics with RejectionError's error.
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
			panic(RejectionError(msg))
		}
		payload = msg.Payload
	case <-timeout.C:
		panic("timeout reached")
	}

	return payload
}
