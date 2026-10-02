/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	server2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type blockingViewManager struct {
	mu         sync.Mutex
	blockFor   time.Duration
	done       chan struct{}
	cancelled  bool
	ranFullDur bool
}

func (f *blockingViewManager) isCancelled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cancelled
}

func (f *blockingViewManager) hasRanFullDur() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ranFullDur
}

func (*blockingViewManager) NewView(_ string, _ []byte) (view2.View, error) {
	return &fakeView{}, nil
}

func (f *blockingViewManager) InitiateView(ctx context.Context, _ view2.View) (any, error) {
	defer close(f.done)
	start := time.Now()
	var cancelled bool
	select {
	case <-ctx.Done():
		cancelled = true
	case <-time.After(f.blockFor):
	}
	f.mu.Lock()
	f.cancelled = cancelled
	f.ranFullDur = time.Since(start) >= f.blockFor
	f.mu.Unlock()
	return []byte("result"), nil
}

func (*blockingViewManager) InitiateContext(_ context.Context, _ view2.View) (view2.Context, error) {
	return nil, nil
}

func (*blockingViewManager) DeleteContext(_ string) {}

type blockingViewContext struct {
	fakeViewContext
	mu       sync.Mutex
	blockFor time.Duration
	done     chan struct{}
	// ctx is set by blockingStreamViewManager.InitiateContext and read by RunView.
	// Both calls happen sequentially on the same goroutine within client.StreamCallView,
	// so no lock is needed for this field.
	ctx        context.Context
	cancelled  bool
	ranFullDur bool
}

func (b *blockingViewContext) isCancelled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cancelled
}

func (b *blockingViewContext) hasRanFullDur() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ranFullDur
}

func (b *blockingViewContext) RunView(_ view2.View, _ ...view2.RunViewOption) (any, error) {
	defer close(b.done)
	start := time.Now()
	var cancelled bool
	select {
	case <-b.ctx.Done():
		cancelled = true
	case <-time.After(b.blockFor):
	}
	b.mu.Lock()
	b.cancelled = cancelled
	b.ranFullDur = time.Since(start) >= b.blockFor
	b.mu.Unlock()
	return []byte("result"), nil
}

type blockingStreamViewManager struct {
	mu           sync.Mutex
	bvc          *blockingViewContext
	deletedCtxID string
}

func (*blockingStreamViewManager) NewView(_ string, _ []byte) (view2.View, error) {
	return &fakeView{}, nil
}

func (*blockingStreamViewManager) InitiateView(_ context.Context, _ view2.View) (any, error) {
	return nil, nil
}

func (m *blockingStreamViewManager) InitiateContext(ctx context.Context, _ view2.View) (view2.Context, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bvc.ctx = ctx
	return m.bvc, nil
}

func (m *blockingStreamViewManager) DeleteContext(contextID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletedCtxID = contextID
}

func (m *blockingStreamViewManager) getDeletedCtxID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deletedCtxID
}

func TestCallView_TimeoutCancelsViewExecution(t *testing.T) {
	t.Parallel()

	const blockFor = 300 * time.Millisecond
	const viewTimeout = 50 * time.Millisecond

	vm := &blockingViewManager{blockFor: blockFor, done: make(chan struct{})}
	ip := &fakeIdentityProvider{}
	tp := noop.NewTracerProvider()
	c := newViewClient(vm, ip, tp, WithTimeout(viewTimeout))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res, err := c.CallView("fid", nil, r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(res.([]byte))
	}))
	t.Cleanup(ts.Close)

	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	resp, err := client.Get(ts.URL) //nolint:noctx
	elapsed := time.Since(start)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	select {
	case <-vm.done:
	case <-time.After(5 * time.Second):
		t.Fatal("InitiateView never returned")
	}

	require.Less(t, elapsed, blockFor,
		"the request should finish before blockFor due to the configured view timeout")
	assert.True(t, vm.isCancelled(),
		"the view's context should have been cancelled by the configured timeout")
	assert.False(t, vm.hasRanFullDur(),
		"the view should not have run for its full blocking duration")
}

func TestCallView_NoTimeoutRunsFullDuration(t *testing.T) {
	t.Parallel()

	const blockFor = 60 * time.Millisecond

	vm := &blockingViewManager{blockFor: blockFor, done: make(chan struct{})}
	ip := &fakeIdentityProvider{}
	tp := noop.NewTracerProvider()
	c := newViewClient(vm, ip, tp, WithTimeout(0))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res, err := c.CallView("fid", nil, r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(res.([]byte))
	}))
	t.Cleanup(ts.Close)

	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	resp, err := client.Get(ts.URL) //nolint:noctx
	elapsed := time.Since(start)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	select {
	case <-vm.done:
	case <-time.After(5 * time.Second):
		t.Fatal("InitiateView never returned")
	}

	require.GreaterOrEqual(t, elapsed, blockFor,
		"without timeout, the request should take at least blockFor duration")
	assert.False(t, vm.isCancelled(),
		"without timeout, the view context should not be cancelled")
	assert.True(t, vm.hasRanFullDur(),
		"without timeout, the view should run for its full configured duration")
}

func TestStreamCallView_TimeoutCancelsViewExecution(t *testing.T) {
	t.Parallel()

	const blockFor = 300 * time.Millisecond
	const viewTimeout = 50 * time.Millisecond

	done := make(chan struct{})
	bvc := &blockingViewContext{blockFor: blockFor, done: done}
	vm := &blockingStreamViewManager{bvc: bvc}
	ip := &fakeIdentityProvider{}
	tp := noop.NewTracerProvider()
	c := newViewClient(vm, ip, tp, WithTimeout(viewTimeout))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = c.StreamCallView("fid", w, r)
	}))
	t.Cleanup(ts.Close)

	wsURL := "ws" + ts.URL[4:]
	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	if resp != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}
	t.Cleanup(func() { _ = ws.Close() })

	inputMsg, err := json.Marshal(server2.Input{Raw: []byte("input")})
	require.NoError(t, err)

	start := time.Now()
	err = ws.WriteMessage(websocket.TextMessage, inputMsg)
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunView never returned")
	}
	elapsed := time.Since(start)

	_, _, err = ws.ReadMessage()
	require.NoError(t, err)

	require.Less(t, elapsed, blockFor,
		"the request should finish before blockFor due to the configured view timeout")
	assert.True(t, bvc.isCancelled(),
		"the view's context should have been cancelled by the configured timeout")
	assert.False(t, bvc.hasRanFullDur(),
		"the view should not have run for its full blocking duration")
	assert.Equal(t, "ctx-id", vm.getDeletedCtxID(),
		"DeleteContext should be called on the view manager after StreamCallView returns")
}

func TestStreamCallView_NoTimeoutRunsFullDuration(t *testing.T) {
	t.Parallel()

	const blockFor = 60 * time.Millisecond

	done := make(chan struct{})
	bvc := &blockingViewContext{blockFor: blockFor, done: done}
	vm := &blockingStreamViewManager{bvc: bvc}
	ip := &fakeIdentityProvider{}
	tp := noop.NewTracerProvider()
	c := newViewClient(vm, ip, tp)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = c.StreamCallView("fid", w, r)
	}))
	t.Cleanup(ts.Close)

	wsURL := "ws" + ts.URL[4:]
	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	if resp != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}
	t.Cleanup(func() { _ = ws.Close() })

	inputMsg, err := json.Marshal(server2.Input{Raw: []byte("input")})
	require.NoError(t, err)

	start := time.Now()
	err = ws.WriteMessage(websocket.TextMessage, inputMsg)
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunView never returned")
	}
	elapsed := time.Since(start)

	_, _, err = ws.ReadMessage()
	require.NoError(t, err)

	require.GreaterOrEqual(t, elapsed, blockFor,
		"without timeout, the stream view should run for at least blockFor duration")
	assert.False(t, bvc.isCancelled(),
		"without timeout, the stream view context should not be cancelled")
	assert.True(t, bvc.hasRanFullDur(),
		"without timeout, the stream view should run for its full duration")
	assert.Equal(t, "ctx-id", vm.getDeletedCtxID(),
		"DeleteContext should be called on the view manager after StreamCallView returns")
}

func TestInstallViewHandler_WithOptions(t *testing.T) {
	t.Parallel()

	vm := &fakeViewManager{}
	ip := &fakeIdentityProvider{}
	tp := noop.NewTracerProvider()
	h := server2.NewHttpHandler()

	InstallViewHandler(vm, ip, h, tp, WithTimeout(10*time.Second))
	require.NotNil(t, h)
}
