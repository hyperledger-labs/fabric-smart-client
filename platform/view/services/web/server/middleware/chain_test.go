/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server/middleware"
)

func TestChain(t *testing.T) {
	t.Parallel()

	// tag returns a middleware that writes "<name>:before," and "<name>:after,"
	// around the next handler.
	tag := func(t *testing.T, name string) middleware.Middleware {
		t.Helper()
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := w.Write([]byte(name + ":before,"))
				assert.NoError(t, err)
				next.ServeHTTP(w, r)
				_, err = w.Write([]byte(name + ":after,"))
				assert.NoError(t, err)
			})
		}
	}
	hello := func(t *testing.T) http.Handler {
		t.Helper()
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("Hello!,"))
			assert.NoError(t, err)
		})
	}
	serve := func(chain middleware.Chain, h http.Handler) string {
		resp := httptest.NewRecorder()
		chain.Handler(h).ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/", nil))
		return resp.Body.String()
	}
	newChain := func(t *testing.T) middleware.Chain {
		t.Helper()
		return middleware.NewChain(tag(t, "1"), tag(t, "2"), tag(t, "3"))
	}

	t.Run("calls middleware in the specified order", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "1:before,2:before,3:before,Hello!,3:after,2:after,1:after,", serve(newChain(t), hello(t)))
	})

	t.Run("calls the handler when the chain is empty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "Hello!,", serve(middleware.NewChain(), hello(t)))
	})

	t.Run("uses the DefaultServeMux when the handler is nil", func(t *testing.T) {
		t.Parallel()
		require.Contains(t, serve(newChain(t), nil), "404 page not found")
	})
}
