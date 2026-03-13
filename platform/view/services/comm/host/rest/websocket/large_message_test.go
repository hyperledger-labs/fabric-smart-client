/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gorilla_websocket "github.com/gorilla/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestOversizedMessageRejection verifies that messages exceeding maxDelimitedPayloadSize (10MB)
// are rejected by the delimitedReader.
func TestOversizedMessageRejection(t *testing.T) {
	testSetup(t)
	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	serverTLSConfig, clientTLSConfig, srcID := testMutualTLSConfigs(t, false)

	srv := startTestServer(t, p, serverTLSConfig, func(s host.P2PStream) {
		// Server just tries to read
		buf := make([]byte, 1024)
		_, _ = s.Read(buf)
	})
	defer srv.Close()

	srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	info := host.StreamInfo{
		RemotePeerID:      "serverID",
		RemotePeerAddress: srvEndpoint,
		ContextID:         "large-ctx",
		SessionID:         "large-sess",
	}

	clientStream, err := p.NewClientStream(info, ctx, srcID, clientTLSConfig)
	require.NoError(t, err)

	// Attempt to send 11MB (exceeds maxDelimitedPayloadSize = 10MB)
	oversizedPayload := make([]byte, 11*1024*1024)

	// Write should fail because it uses the accumulator which checks the size
	_, err = clientStream.Write(oversizedPayload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message header or payload too large")
}

// TestValidBoundaryMessage verifies that a message of exactly 1MB passes.
func TestValidBoundaryMessage(t *testing.T) {
	testSetup(t)
	serverTLSConfig, clientTLSConfig, srcID := testMutualTLSConfigs(t, false)

	receivedChan := make(chan []byte, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := server.OpenWSServerConn(w, r)
		if err != nil {
			return
		}
		// Read raw JSON meta
		var meta StreamMeta
		if err := conn.ReadJSON(&meta); err != nil {
			return
		}
		// Now we have the raw websocket. Read the multiplexed message.
		var mm MultiplexedMessage
		if err := conn.ReadJSON(&mm); err != nil {
			return
		}
		receivedChan <- mm.Msg
	}))
	srv.TLS = serverTLSConfig
	srv.StartTLS()
	defer srv.Close()

	srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")

	dialer := gorilla_websocket.Dialer{TLSClientConfig: clientTLSConfig}
	u := fmt.Sprintf("wss://%s/p2p", srvEndpoint)
	conn, _, err := dialer.Dial(u, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Send meta
	meta := StreamMeta{PeerID: srcID}
	require.NoError(t, conn.WriteJSON(meta))

	// Send exactly 1MB payload inside a MultiplexedMessage
	boundaryPayload := make([]byte, 1*1024*1024)
	boundaryPayload[0] = 0xAA
	boundaryPayload[len(boundaryPayload)-1] = 0xBB

	mm := MultiplexedMessage{ID: "1", Msg: boundaryPayload}
	require.NoError(t, conn.WriteJSON(mm))

	select {
	case received := <-receivedChan:
		assert.NotNil(t, received)
		assert.Equal(t, 1*1024*1024, len(received))
		assert.Equal(t, byte(0xAA), received[0])
		assert.Equal(t, byte(0xBB), received[len(received)-1])
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for 1MB message")
	}
}
