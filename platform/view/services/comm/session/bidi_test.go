/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/
package session

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/require"
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
	select {
	case msg := <-ch.RightSession().Receive():
		require.NotNil(t, msg)
		require.Equal(t, payload, msg.Payload)
		require.EqualValues(t, view.OK, msg.Status)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestLocalBidirectionalChannel_BidirectionalCommunication(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	left := ch.LeftSession()
	right := ch.RightSession()
	require.NoError(t, left.Send([]byte("from left")))
	select {
	case msg := <-right.Receive():
		require.Equal(t, []byte("from left"), msg.Payload)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	require.NoError(t, right.Send([]byte("from right")))
	select {
	case msg := <-left.Receive():
		require.Equal(t, []byte("from right"), msg.Payload)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestLocalBidirectionalChannel_SendError(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	errPayload := []byte("something went wrong")
	require.NoError(t, ch.LeftSession().SendError(errPayload))
	select {
	case msg := <-ch.RightSession().Receive():
		require.EqualValues(t, view.ERROR, msg.Status)
		require.Equal(t, errPayload, msg.Payload)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
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
	require.NoError(t, ch.LeftSession().SendWithContext(context.Background(), payload))
	select {
	case msg := <-ch.RightSession().Receive():
		require.Equal(t, payload, msg.Payload)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestLocalBidirectionalChannel_SendWithCancelledContext(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = ch.LeftSession().SendWithContext(ctx, []byte("data"))
	require.Error(t, err)
}
