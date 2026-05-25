/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/
package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestNewLocalBidirectionalChannel(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "contextID", "endpoint", []byte("pkid"))
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.NotNil(t, ch.LeftSession())
	require.NotNil(t, ch.RightSession())
}

func TestLocalBidirectionalChannel_SendReceive(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	payload := []byte("hello")
	require.NoError(t, ch.LeftSession().Send(payload))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		select {
		case msg := <-ch.RightSession().Receive():
			assert.NotNil(c, msg)
			assert.Equal(c, payload, msg.Payload)
			assert.EqualValues(c, view.OK, msg.Status)
		default:
			assert.Fail(c, "no message received yet")
		}
	}, time.Second, 10*time.Millisecond)
}

func TestLocalBidirectionalChannel_BidirectionalCommunication(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	left := ch.LeftSession()
	right := ch.RightSession()
	require.NoError(t, left.Send([]byte("from left")))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		select {
		case msg := <-right.Receive():
			assert.Equal(c, []byte("from left"), msg.Payload)
		default:
			assert.Fail(c, "no message received yet")
		}
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, right.Send([]byte("from right")))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		select {
		case msg := <-left.Receive():
			assert.Equal(c, []byte("from right"), msg.Payload)
		default:
			assert.Fail(c, "no message received yet")
		}
	}, time.Second, 10*time.Millisecond)
}

func TestLocalBidirectionalChannel_SendError(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	errPayload := []byte("something went wrong")
	require.NoError(t, ch.LeftSession().SendError(errPayload))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		select {
		case msg := <-ch.RightSession().Receive():
			assert.EqualValues(c, view.ERROR, msg.Status)
			assert.Equal(c, errPayload, msg.Payload)
		default:
			assert.Fail(c, "no message received yet")
		}
	}, time.Second, 10*time.Millisecond)
}

func TestLocalBidirectionalChannel_CloseReceive(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	left := ch.LeftSession()
	left.Close()
	require.Nil(t, left.Receive())
}

func TestLocalBidirectionalChannel_SendAfterClose(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	left := ch.LeftSession()
	left.Close()
	err = left.Send([]byte("data"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "session is closed")
}

func TestLocalBidirectionalChannel_SendErrorAfterClose(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	left := ch.LeftSession()
	left.Close()
	err = left.SendError([]byte("data"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "session is closed")
}

func TestLocalBidirectionalChannel_Info(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", []byte("pkid"))
	require.NoError(t, err)
	left := ch.LeftSession()
	info := left.Info()
	require.False(t, info.Closed)
	require.Equal(t, "endpoint", info.Endpoint)
	left.Close()
	require.True(t, left.Info().Closed)
}

func TestLocalBidirectionalChannel_SendWithContext(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	payload := []byte("with context")
	require.NoError(t, ch.LeftSession().SendWithContext(t.Context(), payload))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		select {
		case msg := <-ch.RightSession().Receive():
			assert.Equal(c, payload, msg.Payload)
		default:
			assert.Fail(c, "no message received yet")
		}
	}, time.Second, 10*time.Millisecond)
}

func TestLocalBidirectionalChannel_SendWithCancelledContext(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = ch.LeftSession().SendWithContext(ctx, []byte("data"))
	require.Error(t, err)
}
