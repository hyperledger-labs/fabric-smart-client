/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

const (
	timeout = 100 * time.Millisecond
	tick    = 10 * time.Millisecond
)

type mockSender struct {
}

func (m *mockSender) sendTo(ctx context.Context, info host2.StreamInfo, msg proto.Message) error {
	return nil
}

func setup() *NetworkStreamSession {
	sessionID := "someSessionID"
	contextID := "someContextID"
	endpointAddress := "someEndpointAddress"
	endpointID := []byte("someEndpointID")
	caller := []byte("me")
	callerViewID := "someviewID"

	net := &mockSender{}

	ctx, cancel := context.WithCancel(context.Background())
	return &NetworkStreamSession{
		node:            net,
		endpointID:      endpointID,
		endpointAddress: endpointAddress,
		contextID:       contextID,
		sessionID:       sessionID,
		caller:          caller,
		callerViewID:    callerViewID,
		incoming:        make(chan *view.Message, 1),
		streams:         make(map[*streamHandler]struct{}),
		ctx:             ctx,
		cancel:          cancel,
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := setup()

	// hide the impl behind the session interface as a consumer
	var sess view.Session = s

	require.False(t, sess.Info().Closed)

	require.NoError(t, sess.Send([]byte("some message")))
	require.NoError(t, sess.SendError([]byte("some error")))

	msg := &view.Message{
		Payload: []byte("some message"),
	}

	// enqueue a message
	require.Empty(t, s.incoming)
	s.enqueue(msg)
	require.Len(t, s.incoming, 1)

	// we should receive this message
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		recvMsg := <-sess.Receive()
		require.Equal(c, msg, recvMsg)
	}, timeout, tick)
	require.Empty(t, s.incoming)

	// let's wrap up
	sess.Close()
	require.True(t, sess.Info().Closed)

	// sending on closed session should return an error
	require.ErrorIs(t, sess.Send([]byte("some message")), ErrSessionClosed)
	require.ErrorIs(t, sess.SendError([]byte("some error")), ErrSessionClosed)

	// enqueue on closed session should just drop the message
	require.Empty(t, s.incoming)
	s.enqueue(msg)
	require.Empty(t, s.incoming)

	// on a closed session, a reader should return immediately
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var cnt int
		for range sess.Receive() {
			// since the session is closed the range loop should not be invoked at all
			cnt++
		}
		require.Equal(c, 0, cnt)
	}, timeout, tick)

	// survive another close
	sess.Close()
}

func TestSessionLifecycleConcurrent(t *testing.T) {
	// let check that at the end of this test all our go routines are stopped
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	s := setup()

	// hide the impl behind the session interface as a consumer
	var sess view.Session = s

	const numMessage = 100
	var wg sync.WaitGroup

	// sender
	wg.Add(1)
	go func() {
		defer wg.Done()

		// send a few messages
		for i := range numMessage {
			s.enqueue(&view.Message{Payload: []byte(fmt.Sprintf("msg #%v", i))})
		}

		// once we delivered all our messages we close
		sess.Close()

		// try to send more but the session should not accept them
		for i := range numMessage {
			s.enqueue(&view.Message{Payload: []byte(fmt.Sprintf("msg #%v", i))})
			assert.Empty(t, s.incoming)
		}
	}()

	// consumer
	wg.Add(1)
	go func() {
		defer wg.Done()

		cnt := 0
		for msg := range sess.Receive() {
			assert.Equal(t, fmt.Sprintf("msg #%v", cnt), string(msg.Payload))
			cnt = cnt + 1
		}
		assert.Equal(t, numMessage, cnt)
		assert.True(t, sess.Info().Closed)
	}()

	wg.Wait()
}
