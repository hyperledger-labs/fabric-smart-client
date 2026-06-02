/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

func TestWithRecovery(t *testing.T) {
	t.Parallel()

	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	})

	h := withRecovery(panicking)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/p2p", nil)
	require.NotPanics(t, func() { h.ServeHTTP(rec, req) })
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestWithLogging(t *testing.T) {
	t.Parallel()

	t.Run("2xx does not log error", func(t *testing.T) {
		t.Parallel()
		ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := withLogging(ok)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/p2p", nil)
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("4xx still completes the request", func(t *testing.T) {
		t.Parallel()
		notFound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		h := withLogging(notFound)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/missing", nil)
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("implicit 200 when WriteHeader is never called", func(t *testing.T) {
		t.Parallel()
		silent := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		h := withLogging(silent)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/p2p", nil)
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestWithCORS(t *testing.T) {
	t.Parallel()

	allowed := []string{"https://example.com"}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("allowed origin sets CORS headers", func(t *testing.T) {
		t.Parallel()
		h := withCORS(allowed, inner)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/p2p", nil)
		req.Header.Set("Origin", "https://example.com")
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
		require.NotEmpty(t, rec.Header().Get("Access-Control-Allow-Headers"))
	})

	t.Run("disallowed origin does not set CORS headers", func(t *testing.T) {
		t.Parallel()
		h := withCORS(allowed, inner)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/p2p", nil)
		req.Header.Set("Origin", "https://evil.com")
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("no origin header passes through untouched", func(t *testing.T) {
		t.Parallel()
		h := withCORS(allowed, inner)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/p2p", nil)
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("OPTIONS preflight returns 204 with method list", func(t *testing.T) {
		t.Parallel()
		h := withCORS(allowed, inner)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/p2p", nil)
		req.Header.Set("Origin", "https://example.com")
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
		require.NotEmpty(t, rec.Header().Get("Access-Control-Allow-Methods"))
	})
}

func TestNewHandler_Routing(t *testing.T) {
	t.Parallel()

	var called bool
	provider := &fakeStreamProvider{fn: func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusSwitchingProtocols)
	}}

	h := newHandler(provider, nil, nil)

	t.Run("GET /p2p is routed", func(t *testing.T) {
		t.Parallel()
		called = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/p2p", nil)
		h.ServeHTTP(rec, req)
		require.True(t, called)
	})

	t.Run("unknown path returns 404", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("POST /p2p returns 405", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/p2p", nil)
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}

type fakeStreamProvider struct {
	fn func(http.ResponseWriter, *http.Request)
}

func (f *fakeStreamProvider) NewServerStream(w http.ResponseWriter, r *http.Request, _ func(host2.P2PStream)) error {
	if f.fn != nil {
		f.fn(w, r)
	}
	return nil
}
