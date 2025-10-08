/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestAttack_SpoofPeerID attempts to reproduce the attack where an attacker
// connects with their own certificate but claims a different PeerID in the meta message.
// This is a regression test for Issue #1037.
func TestAttack_SpoofPeerID(t *testing.T) {
	testSetup(t)
	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	serverTLSConfig, clientTLSConfig, _ := testMutualTLSConfigs(t, false)

	received := make(chan host.P2PStream, 1)
	srv := startTestServer(t, p, serverTLSConfig, func(s host.P2PStream) {
		received <- s
	})
	defer srv.Close()

	srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")

	// Alice is the victim, Charlie is the attacker.
	// Charlie connects with his own cert but claims to be Alice.
	attackerPeerID := host.PeerID("Alice-ID")

	dialer := websocket.Dialer{
		TLSClientConfig: clientTLSConfig,
	}
	u := fmt.Sprintf("wss://%s/p2p", srvEndpoint)
	conn, _, err := dialer.Dial(u, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Send meta message claiming to be Alice
	meta := StreamMeta{
		SessionID: "S1",
		ContextID: "C1",
		PeerID:    attackerPeerID,
	}
	mm := MultiplexedMessage{
		ID:  "1",
		Msg: mustMarshal(meta),
	}
	require.NoError(t, conn.WriteJSON(mm))

	// According to the fix, the server should reject this and close the connection.
	// We try to read a message back, which should be an error or a specific error message.
	var response MultiplexedMessage
	err = conn.ReadJSON(&response)
	if err == nil {
		assert.Equal(t, "peer identity binding failed", response.Err)
	}

	// Verify that the server didn't accept the stream
	select {
	case <-received:
		t.Fatal("Server accepted a stream with spoofed PeerID")
	case <-time.After(500 * time.Millisecond):
		// Success: server did not call newStreamCallback
	}
}

// TestAttack_HijackSessionID attempts to reproduce the attack where an attacker
// uses a legitimate PeerID (their own) but tries to inject a message into someone else's SessionID.
// This is a regression test for Issue #1037.
func TestAttack_HijackSessionID(t *testing.T) {
	testSetup(t)
	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	serverTLSConfig, clientTLSConfig, charlieID := testMutualTLSConfigs(t, false)

	received := make(chan host.P2PStream, 2)
	srv := startTestServer(t, p, serverTLSConfig, func(s host.P2PStream) {
		received <- s
	})
	defer srv.Close()

	srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")

	dialer := websocket.Dialer{
		TLSClientConfig: clientTLSConfig,
	}
	u := fmt.Sprintf("wss://%s/p2p", srvEndpoint)
	conn, _, err := dialer.Dial(u, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Charlie connects legitimately
	meta := StreamMeta{
		SessionID: "Alice-Session-ID", // Charlie knows Alice's SessionID
		ContextID: "C1",
		PeerID:    charlieID,
	}
	mm := MultiplexedMessage{
		ID:  "1",
		Msg: mustMarshal(meta),
	}
	require.NoError(t, conn.WriteJSON(mm))

	// Server should accept Charlie's stream because meta.PeerID matches cert
	var srvStream host.P2PStream
	select {
	case srvStream = <-received:
		// Success: server accepted Charlie's stream
	case <-time.After(time.Second):
		t.Fatal("Server did not accept Charlie's stream")
	}

	// Verify that the RemotePeerID of the stream is indeed Charlie
	assert.Equal(t, string(charlieID), srvStream.RemotePeerID())

	// Note: In a full system, P2PNode uses internalSessionID = SessionID + "." + hash(FromPKID).
	// Since FromPKID is verified by the host (Charlie), Charlie's internal session ID
	// for "Alice-Session-ID" will be "Alice-Session-ID.Charlie", which is isolated
	// from Alice's internal session ID "Alice-Session-ID.Alice".
}

func startTestServer(t *testing.T, p *MultiplexedProvider, tlsConfig *tls.Config, newStreamCallback func(s host.P2PStream)) *httptest.Server {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := p.NewServerStream(w, r, newStreamCallback)
		assert.NoError(t, err)
	}))
	srv.TLS = tlsConfig
	srv.StartTLS()
	return srv
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
