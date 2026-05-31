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

func newTestJSONSession(t *testing.T, ch chan *view.Message) *jsonSession {
	t.Helper()
	s := &mockSession{ch: ch}
	return newJSONSession(s, t.Context())
}

func TestJSONSession_ReceiveSuccess(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message, 1)
	type payload struct{ Name string }
	data := `{"Name":"test"}`
	ch <- &view.Message{Payload: []byte(data), Status: view.OK}
	js := newTestJSONSession(t, ch)
	var result payload
	require.NoError(t, js.Receive(&result))
	require.Equal(t, "test", result.Name)
}

func TestJSONSession_ReceiveTimeout(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message)
	js := newTestJSONSession(t, ch)
	err := js.ReceiveWithTimeout(nil, 50*time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "time out reached")
}

func TestJSONSession_ReceiveErrorMessage(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message, 1)
	ch <- &view.Message{Payload: []byte("remote error"), Status: view.ERROR}
	js := newTestJSONSession(t, ch)
	var result interface{}
	err := js.Receive(&result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "received error from remote")
}

func TestJSONSession_ReceiveNilMessage(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message, 1)
	ch <- nil
	js := newTestJSONSession(t, ch)
	var result interface{}
	err := js.Receive(&result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "received message is nil")
}

func TestJSONSession_ReceiveInvalidJSON(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message, 1)
	ch <- &view.Message{Payload: []byte("not-json"), Status: view.OK}
	js := newTestJSONSession(t, ch)
	var result map[string]string
	err := js.Receive(&result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed unmarshalling state")
}

func TestJSONSession_Send(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	js := newJSONSession(ch.LeftSession(), t.Context())
	type payload struct{ Name string }
	require.NoError(t, js.Send(payload{Name: "hello"}))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		select {
		case msg := <-ch.RightSession().Receive():
			assert.Contains(c, string(msg.Payload), "hello")
		default:
			assert.Fail(c, "no message received yet")
		}
	}, time.Second, 10*time.Millisecond)
}

func TestJSONSession_SendError(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	js := newJSONSession(ch.LeftSession(), t.Context())
	require.NoError(t, js.SendError("something failed"))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		select {
		case msg := <-ch.RightSession().Receive():
			assert.EqualValues(c, view.ERROR, msg.Status)
		default:
			assert.Fail(c, "no message received yet")
		}
	}, time.Second, 10*time.Millisecond)
}

func TestJSONSession_SendRaw(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	js := newJSONSession(ch.LeftSession(), t.Context())
	raw := []byte("raw bytes")
	require.NoError(t, js.SendRaw(t.Context(), raw))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		select {
		case msg := <-ch.RightSession().Receive():
			assert.Equal(c, raw, msg.Payload)
		default:
			assert.Fail(c, "no message received yet")
		}
	}, time.Second, 10*time.Millisecond)
}

func TestJSONSession_Info(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", []byte("pkid"))
	require.NoError(t, err)
	js := newJSONSession(ch.LeftSession(), t.Context())
	info := js.Info()
	require.Equal(t, "endpoint", info.Endpoint)
}

func TestJSONSession_Session(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	left := ch.LeftSession()
	js := newJSONSession(left, t.Context())
	require.Equal(t, left, js.Session())
}

func TestJSONSession_ContextCancelled(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message)
	ctx, cancel := context.WithCancel(t.Context())
	js := newJSONSession(&mockSession{ch: ch}, ctx)
	cancel()
	var result interface{}
	err := js.ReceiveWithTimeout(&result, time.Second)
	require.Error(t, err)
}

func TestJSONSession_ReceiveRaw(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message, 1)
	ch <- &view.Message{Payload: []byte("raw"), Status: view.OK}
	js := newTestJSONSession(t, ch)
	raw, err := js.ReceiveRaw()
	require.NoError(t, err)
	require.Equal(t, []byte("raw"), raw)
}

func TestNewFromSession(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	msgCh := make(chan *view.Message, 1)
	ctx := &mockContext{s: &mockSession{ch: msgCh}}
	js := NewFromSession(ctx, ch.LeftSession())
	require.NotNil(t, js)
}

func TestJSON(t *testing.T) {
	t.Parallel()
	msgCh := make(chan *view.Message, 1)
	ctx := &mockContext{s: &mockSession{ch: msgCh}}
	js := JSON(ctx)
	require.NotNil(t, js)
}
