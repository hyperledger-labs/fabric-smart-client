/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSameOriginOrNonBrowser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{name: "no origin", host: "example.com", want: true},
		{name: "same host", origin: "http://example.com", host: "example.com", want: true},
		{name: "same host different case", origin: "http://example.com", host: "EXAMPLE.com", want: true},
		{name: "different host", origin: "http://evil.com", host: "example.com", want: false},
		{name: "unparsable origin", origin: "://bad", host: "example.com", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			assert.Equal(t, tc.want, sameOriginOrNonBrowser(req))
		})
	}
}
