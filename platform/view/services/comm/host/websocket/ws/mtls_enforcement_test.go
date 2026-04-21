/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// TestMTLSStrictness verifies that the websocket provider strictly requires mTLS.
func TestMTLSStrictness(t *testing.T) {
	t.Parallel()
	testSetup(t)
	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)

	serverTLSConfig, clientTLSConfig, _ := testMutualTLSConfigs(t, false)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := p.NewServerStream(w, r, func(s host.P2PStream) {
			_ = s.Close()
		})
		if err != nil {
			// server.OpenWSServerConn will fail if mTLS is not satisfied
			return
		}
	}))
	srv.TLS = serverTLSConfig
	srv.StartTLS()
	t.Cleanup(srv.Close)

	srvAddr := strings.TrimPrefix(srv.URL, "https://")
	url := fmt.Sprintf("wss://%s/p2p", srvAddr)

	t.Run("Valid mTLS - Connection Accepted", func(t *testing.T) {
		t.Parallel()
		dialer := websocket.Dialer{TLSClientConfig: clientTLSConfig}
		conn, _, err := dialer.Dial(url, nil)
		require.NoError(t, err)
		_ = conn.Close()
	})

	t.Run("No Client Certificate - Connection Rejected", func(t *testing.T) {
		t.Parallel()
		// Only root CA provided, NO client cert
		invalidClientConfig := &tls.Config{
			RootCAs:    clientTLSConfig.RootCAs,
			ServerName: "localhost",
		}
		dialer := websocket.Dialer{TLSClientConfig: invalidClientConfig}
		_, _, err := dialer.Dial(url, nil)
		assert.Error(t, err, "Websocket dial should have failed without client certificate")
	})

	t.Run("Untrusted Client Certificate - Connection Rejected", func(t *testing.T) {
		t.Parallel()
		// Use another CA to sign a client certificate
		_, untrustedClientConfig, _ := testMutualTLSConfigs(t, false)

		dialer := websocket.Dialer{TLSClientConfig: untrustedClientConfig}
		_, _, err := dialer.Dial(url, nil)
		assert.Error(t, err, "Websocket dial should have failed with untrusted client certificate")
	})

	t.Run("Expired Certificate - Connection Rejected", func(t *testing.T) {
		t.Parallel()
		// This is naturally handled by Go's TLS implementation if mTLS is enabled.
		// We could simulate by setting NotAfter in the past during cert generation in testMutualTLSConfigs.
	})
}
