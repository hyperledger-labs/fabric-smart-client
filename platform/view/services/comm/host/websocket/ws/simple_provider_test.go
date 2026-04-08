/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/stretchr/testify/require"
)

func TestSimpleProvider_Security(t *testing.T) {
	testSetup(t)

	for _, insecureSkipVerify := range []bool{true, false} {
		mode := fmt.Sprintf("InsecureSkipVerify=%v", insecureSkipVerify)
		t.Run(mode, func(t *testing.T) {
			p := NewSimpleProvider()
			serverTLSConfig, clientTLSConfig, srcID := testMutualTLSConfigs(t, insecureSkipVerify)

			received := make(chan host.P2PStream, 1)
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = p.NewServerStream(w, r, func(s host.P2PStream) {
					received <- s
				})
			}))
			srv.TLS = serverTLSConfig
			srv.StartTLS()
			defer srv.Close()

			srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")

			t.Run("successful connection and verified RemotePeerID", func(t *testing.T) {
				info := host.StreamInfo{
					RemotePeerID:      "serverID",
					RemotePeerAddress: srvEndpoint,
					ContextID:         "ctx",
					SessionID:         "sess",
				}

				client, err := p.NewClientStream(info, t.Context(), srcID, clientTLSConfig)
				require.NoError(t, err)
				defer func() { _ = client.Close() }()

				select {
				case s := <-received:
					require.Equal(t, srcID, s.RemotePeerID(), "RemotePeerID should match authenticated identity")
				case <-time.After(5 * time.Second):
					t.Fatal("timeout waiting for server stream")
				}
			})

			t.Run("reject spoofed PeerID", func(t *testing.T) {
				// Attacker tries to claim they are "Alice-ID"
				spoofedID := host.PeerID("Alice-ID")

				// We need to bypass NewClientStream because it uses the real srcID
				// Let's use a raw dialer to send a spoofed meta message
				dialer := websocket.Dialer{
					TLSClientConfig: clientTLSConfig,
				}
				u := fmt.Sprintf("wss://%s/p2p", srvEndpoint)
				conn, _, err := dialer.Dial(u, nil)
				require.NoError(t, err)
				defer func() { _ = conn.Close() }()

				meta := StreamMeta{
					ContextID: "ctx",
					SessionID: "sess2",
					PeerID:    spoofedID,
				}
				require.NoError(t, conn.WriteJSON(meta))

				// Server should reject and close connection
				_, _, err = conn.ReadMessage()
				require.Error(t, err, "Server should have closed connection")

				select {
				case <-received:
					t.Fatal("server accepted a stream with spoofed peer ID")
				case <-time.After(500 * time.Millisecond):
					// Success
				}
			})
		})
	}
}
