/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeServer is a minimal Server implementation whose Start blocks until
// Stop is called, so tests can observe whether Serve/serve waits for the
// server goroutine to actually finish before returning.
type fakeServer struct {
	stopCh    chan struct{}
	closeOnce sync.Once
	stoppedFl atomic.Bool
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		stopCh: make(chan struct{}),
	}
}

func (f *fakeServer) RegisterHandler(_ string, _ http.Handler, _ bool) {}

func (f *fakeServer) Start() error {
	<-f.stopCh
	return nil
}

func (f *fakeServer) Stop() error {
	f.closeOnce.Do(func() {
		f.stoppedFl.Store(true)
		close(f.stopCh)
	})
	return nil
}

func (f *fakeServer) stopped() bool {
	return f.stoppedFl.Load()
}

func TestServe_WaitsForServersOnShutdown(t *testing.T) { //nolint:paralleltest // relies on server-goroutine shutdown timing; must run serially
	ctx, cancel := context.WithCancel(context.Background())
	ws := newFakeServer() // Start blocks until Stop
	// Zero OperationsServer: the endpoints share the web listener, so serve starts none.
	wg := serve(nil, ws, OperationsServer{}, nil, nil, ctx)
	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not join server goroutines on shutdown")
	}
	require.True(t, ws.stopped())
}
