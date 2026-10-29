/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server/middleware"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server/middleware/fakes"
)

func TestWithRequestID(t *testing.T) {
	t.Parallel()

	// serve passes a request through WithRequestID, with the X-Request-Id
	// header set to receivedID unless it is empty. It returns the request the
	// next handler saw and the recorded response.
	serve := func(t *testing.T, receivedID string) (*http.Request, *httptest.ResponseRecorder) {
		t.Helper()
		handler := &fakes.HTTPHandler{}
		chain := middleware.WithRequestID(
			middleware.GenerateIDFunc(func() string { return "generated-id" }),
		)(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if receivedID != "" {
			req.Header.Set("X-Request-Id", receivedID)
		}
		resp := httptest.NewRecorder()
		chain.ServeHTTP(resp, req)

		require.Equal(t, 1, handler.ServeHTTPCallCount())
		_, r := handler.ServeHTTPArgsForCall(0)
		return r, resp
	}

	t.Run("propagates the generated request ID in the request context", func(t *testing.T) {
		t.Parallel()
		r, _ := serve(t, "")
		require.Equal(t, "generated-id", middleware.RequestID(r.Context()))
	})

	t.Run("returns the generated request ID in a header", func(t *testing.T) {
		t.Parallel()
		_, resp := serve(t, "")
		require.Equal(t, "generated-id", resp.Result().Header.Get("X-Request-Id"))
	})

	t.Run("when a request ID is already present", func(t *testing.T) {
		t.Parallel()

		t.Run("sets the received ID into the context", func(t *testing.T) {
			t.Parallel()
			r, _ := serve(t, "received-id")
			require.Equal(t, "received-id", middleware.RequestID(r.Context()))
		})

		t.Run("sets the received ID into the request", func(t *testing.T) {
			t.Parallel()
			r, _ := serve(t, "received-id")
			require.Equal(t, "received-id", r.Header.Get("X-Request-Id"))
		})

		t.Run("propagates the request ID to the response", func(t *testing.T) {
			t.Parallel()
			_, resp := serve(t, "received-id")
			require.Equal(t, "received-id", resp.Result().Header.Get("X-Request-Id"))
		})
	})
}
