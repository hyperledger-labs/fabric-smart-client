/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/
package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func newTestJSONSession(ch chan *view.Message) *jsonSession {
	s := &mockSession{ch: ch}
	return newJSONSession(s, context.Background())
}

func TestJSONSession_ReceiveSuccess(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message, 1)
	type payload struct{ Name string }
	data := `{"Name":"test"}`
	ch <- &view.Message{Payload: []byte(data), Status: view.OK}
	js := newTestJSONSession(ch)
	var result payload
	require.NoError(t, js.Receive(&result))
	require.Equal(t, "test", result.Name)
}

func TestJSONSession_ReceiveTimeout(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message)
	js := newTestJSONSession(ch)
	err := js.ReceiveWithTimeout(nil, 50*time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "time out reached")
}

func TestJSONSession_ReceiveErrorMessage(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message, 1)
	ch <- &view.Message{Payload: []byte("remote error"), Status: view.ERROR}
	js := newTestJSONSession(ch)
	var result interface{}
	err := js.Receive(&result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "received error from remote")
}

func TestJSONSession_ReceiveNilMessage(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message, 1)
	ch <- nil
	js := newTestJSONSession(ch)
	var result interface{}
	err := js.Receive(&result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "received message is nil")
}

func TestJSONSession_ReceiveInvalidJSON(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message, 1)
	ch <- &view.Message{Payload: []byte("not-json"), Status: view.OK}
	js := newTestJSONSession(ch)
	var result map[string]string
	err := js.Receive(&result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed unmarshalling state")
}

func TestJSONSession_Send(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	js := newJSONSession(ch.LeftSession(), context.Background())
	type payload struct{ Name string }
	require.NoError(t, js.Send(payload{Name: "hello"}))
	select {
	case msg := <-ch.RightSession().Receive():
		require.Contains(t, string(msg.Payload), "hello")
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestJSONSession_SendError(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	js := newJSONSession(ch.LeftSession(), context.Background())
	require.NoError(t, js.SendError("something failed"))
	select {
	case msg := <-ch.RightSession().Receive():
		require.EqualValues(t, view.ERROR, msg.Status)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestJSONSession_SendRaw(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	js := newJSONSession(ch.LeftSession(), context.Background())
	raw := []byte("raw bytes")
	require.NoError(t, js.SendRaw(context.Background(), raw))
	select {
	case msg := <-ch.RightSession().Receive():
		require.Equal(t, raw, msg.Payload)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestJSONSession_Info(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", []byte("pkid"))
	require.NoError(t, err)
	js := newJSONSession(ch.LeftSession(), context.Background())
	info := js.Info()
	require.Equal(t, "endpoint", info.Endpoint)
}

func TestJSONSession_Session(t *testing.T) {
	t.Parallel()
	ch, err := NewLocalBidirectionalChannel("caller", "ctx", "endpoint", nil)
	require.NoError(t, err)
	left := ch.LeftSession()
	js := newJSONSession(left, context.Background())
	require.Equal(t, left, js.Session())
}

func TestJSONSession_ContextCancelled(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message)
	ctx, cancel := context.WithCancel(context.Background())
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
	js := newTestJSONSession(ch)
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
