/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package session

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ReadMessageWithTimeout returns the payload of the next message on session, waiting at most d.
// An error reply from the remote end is returned as RejectionError's error. It does not
// observe a context; ReadMessageWithContext does, and a view should use it so that it ends
// when its caller cancels.
func ReadMessageWithTimeout(session Session, d time.Duration) ([]byte, error) {
	return ReadMessageWithContext(context.Background(), session, d)
}

// ReadMessageWithContext is ReadMessageWithTimeout that also fails with ctx's error as soon
// as ctx ends. A view passes its view context, so that a caller that cancels the view, or
// whose deadline expires, ends a view waiting for a reply at once; the view then closes its
// sessions, which ends the responder on the other node, instead of both waiting for d.
func ReadMessageWithContext(ctx context.Context, session Session, d time.Duration) ([]byte, error) {
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
	case <-ctx.Done():
		return nil, errors.Wrap(ctx.Err(), "stopped waiting for a message")
	case <-timeout.C:
		return nil, errors.New("time out reached")
	}

	return payload, nil
}

// ReadFirstMessage returns viewCtx's session and the payload of its next message, waiting at
// most 30 seconds, or until viewCtx's context ends. An error reply from the remote end is
// returned as RejectionError's error.
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
	case <-viewCtx.Context().Done():
		return nil, nil, errors.Wrap(viewCtx.Context().Err(), "stopped waiting for the first message")
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
	case <-viewCtx.Context().Done():
		panic(errors.Wrap(viewCtx.Context().Err(), "stopped waiting for the first message"))
	case <-timeout.C:
		panic("timeout reached")
	}

	return payload
}
