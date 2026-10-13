/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerTimeouts(t *testing.T) {
	t.Parallel()

	s := NewServer(Options{ListenAddress: "127.0.0.1:0"})
	assert.Equal(t, 10*time.Second, s.httpServer.ReadHeaderTimeout)
	assert.Equal(t, 10*time.Second, s.httpServer.ReadTimeout)
	assert.Equal(t, 2*time.Minute, s.httpServer.WriteTimeout)
	assert.Equal(t, 2*time.Minute, s.httpServer.IdleTimeout)
}

// startWithHandler serves h on a started server and issues one request, returning once h
// has been entered.
func startWithHandler(t *testing.T, h func(http.ResponseWriter, *http.Request)) *Server {
	t.Helper()
	s := NewServer(Options{ListenAddress: "127.0.0.1:0"})
	entered := make(chan struct{})
	s.RegisterHandler("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		h(w, r)
	}), false)
	require.NoError(t, s.Start())
	go func() {
		if resp, err := http.Get("http://" + s.Addr() + "/"); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-entered
	return s
}

func stopAsync(s *Server) <-chan error {
	stopped := make(chan error, 1)
	go func() { stopped <- s.Stop() }()
	return stopped
}

func setStopTimeouts(t *testing.T, grace, wait time.Duration) {
	t.Helper()
	oldGrace, oldWait := shutdownGracePeriod, handlerWaitTimeout
	shutdownGracePeriod, handlerWaitTimeout = grace, wait
	t.Cleanup(func() { shutdownGracePeriod, handlerWaitTimeout = oldGrace, oldWait })
}

// http.Server.Shutdown does not track hijacked connections, so this covers the handler
// wait on its own.
func TestStopWaitsForHijackedHandler(t *testing.T) { //nolint:paralleltest // relies on shutdown timing
	release := make(chan struct{})
	s := startWithHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := http.NewResponseController(w).Hijack()
		if err == nil {
			defer func() { _ = conn.Close() }()
		}
		<-release
	})

	stopped := stopAsync(s)
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before the handler did: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-stopped)
}

func TestStopCancelsHandlersAfterGracePeriod(t *testing.T) { //nolint:paralleltest // overrides the stop timeouts
	setStopTimeouts(t, 50*time.Millisecond, 5*time.Second)
	s := startWithHandler(t, func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	require.NoError(t, s.Stop())
}

func TestStopGivesUpOnHandlerIgnoringCancellation(t *testing.T) { //nolint:paralleltest // overrides the stop timeouts
	setStopTimeouts(t, 50*time.Millisecond, 50*time.Millisecond)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	s := startWithHandler(t, func(http.ResponseWriter, *http.Request) { <-release })
	require.ErrorContains(t, s.Stop(), "in-flight handlers did not return")
}

func TestStopRejectsRequestsOnceStopping(t *testing.T) {
	t.Parallel()

	s := NewServer(Options{ListenAddress: "127.0.0.1:0"})
	s.stopping = true
	rec := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
