/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package middleware_test

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server/middleware"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server/middleware/fakes"
)

func TestRequireCert(t *testing.T) {
	t.Parallel()

	// serve passes an HTTPS request through RequireCert after letting setTLS
	// change its connection state. It returns the response status code and
	// how often the next handler ran.
	serve := func(t *testing.T, setTLS func(r *http.Request)) (status, calls int) {
		t.Helper()
		handler := &fakes.HTTPHandler{}
		req := httptest.NewRequest(http.MethodGet, "https:///", nil)
		setTLS(req)
		resp := httptest.NewRecorder()
		middleware.RequireCert()(handler).ServeHTTP(resp, req)
		return resp.Result().StatusCode, handler.ServeHTTPCallCount()
	}
	withChains := func(chains [][]*x509.Certificate) func(*http.Request) {
		return func(r *http.Request) { r.TLS.VerifiedChains = chains }
	}

	t.Run("delegates to the next handler when the first verified chain is not empty", func(t *testing.T) {
		t.Parallel()
		status, calls := serve(t, withChains([][]*x509.Certificate{{&x509.Certificate{}}}))
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, 1, calls)
	})

	for _, tc := range []struct {
		name   string
		setTLS func(*http.Request)
	}{
		{"when the TLS connection state is nil", func(r *http.Request) { r.TLS = nil }},
		{"when verified chains is nil", withChains(nil)},
		{"when verified chains is empty", withChains([][]*x509.Certificate{})},
		{"when the first verified chain is empty", withChains([][]*x509.Certificate{{}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			t.Run("responds with http.StatusUnauthorized", func(t *testing.T) {
				t.Parallel()
				status, _ := serve(t, tc.setTLS)
				require.Equal(t, http.StatusUnauthorized, status)
			})

			t.Run("does not call the next handler", func(t *testing.T) {
				t.Parallel()
				_, calls := serve(t, tc.setTLS)
				require.Equal(t, 0, calls)
			})
		})
	}
}
