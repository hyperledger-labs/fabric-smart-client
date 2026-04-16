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
	"go.opentelemetry.io/otel/trace"
)

type mockSession struct {
	ch <-chan *view.Message
}

func (m *mockSession) Info() view.SessionInfo { return view.SessionInfo{} }

func (m *mockSession) Send([]byte) error { return nil }

func (m *mockSession) SendWithContext(context.Context, []byte) error { return nil }

func (m *mockSession) SendError([]byte) error { return nil }

func (m *mockSession) SendErrorWithContext(context.Context, []byte) error { return nil }

func (m *mockSession) Receive() <-chan *view.Message { return m.ch }

func (m *mockSession) Close() {}

type mockContext struct {
	s view.Session
}

func (m *mockContext) ID() string              { return "" }
func (m *mockContext) Me() view.Identity       { return nil }
func (m *mockContext) IsMe(view.Identity) bool { return false }
func (m *mockContext) Initiator() view.View    { return nil }
func (m *mockContext) GetSession(view.View, view.Identity, ...view.View) (view.Session, error) {
	return nil, nil
}

func (m *mockContext) GetSessionByID(string, view.Identity) (view.Session, error) {
	return nil, nil
}
func (m *mockContext) Context() context.Context { return context.Background() }
func (m *mockContext) Session() view.Session    { return m.s }
func (m *mockContext) RunView(view.View, ...view.RunViewOption) (interface{}, error) {
	return nil, nil
}
func (m *mockContext) OnError(func()) {}
func (m *mockContext) GetService(interface{}) (interface{}, error) {
	return nil, nil
}

func (m *mockContext) StartSpanFrom(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}

func TestReadMessageWithTimeoutClosedChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message)
	close(ch)

	_, err := ReadMessageWithTimeout(&mockSession{ch: ch}, 50*time.Millisecond)
	require.EqualError(t, err, "session receive channel is closed")
}

func TestReadFirstMessageClosedChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message)
	close(ch)

	ctx := &mockContext{s: &mockSession{ch: ch}}
	_, _, err := ReadFirstMessage(ctx)
	require.EqualError(t, err, "session receive channel is closed")
}

func TestReadMessageWithTimeoutNilChannel(t *testing.T) {
	t.Parallel()
	_, err := ReadMessageWithTimeout(&mockSession{ch: nil}, 50*time.Millisecond)
	require.EqualError(t, err, "session receive channel is nil")
}

func TestReadFirstMessageOrPanicClosedChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan *view.Message)
	close(ch)

	ctx := &mockContext{s: &mockSession{ch: ch}}
	require.PanicsWithValue(t, "session receive channel is closed", func() {
		_ = ReadFirstMessageOrPanic(ctx)
	})
}
