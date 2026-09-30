/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

func TestSimpleProvider_Security(t *testing.T) { //nolint:paralleltest
	testSetup(t)

	for _, insecureSkipVerify := range []bool{true, false} { //nolint:paralleltest
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
			t.Cleanup(srv.Close)

			srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")

			t.Run("successful connection and verified RemotePeerID", func(t *testing.T) { //nolint:paralleltest
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

			t.Run("reject spoofed PeerID", func(t *testing.T) { //nolint:paralleltest
				// Attacker tries to claim they are "Alice-ID"
				spoofedID := host.PeerID("Alice-ID")

				// We need to bypass NewClientStream because it uses the real srcID
				// Let's use a raw dialer to send a spoofed meta message
				dialer := websocket.Dialer{
					TLSClientConfig: clientTLSConfig,
				}
				u := fmt.Sprintf("wss://%s/p2p", srvEndpoint)
				conn, resp, err := dialer.Dial(u, nil)
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()
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

func TestSimpleProviderStreamErrors(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	p := NewSimpleProvider()
	require.NoError(t, p.Close())
	requireServerStreamErrors(t, p)

	serverTLSConfig, clientTLSConfig, srcID := testMutualTLSConfigs(t, false)
	errs := make(chan error, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errs <- p.NewServerStream(w, r, func(host.P2PStream) { t.Error("unexpected stream") })
	}))
	srv.TLS = serverTLSConfig
	srv.StartTLS()
	t.Cleanup(srv.Close)
	dial := func() *websocket.Conn {
		conn, resp, err := (&websocket.Dialer{TLSClientConfig: clientTLSConfig}).Dial("wss://"+strings.TrimPrefix(srv.URL, "https://")+"/p2p", nil)
		require.NoError(t, err)
		_ = resp.Body.Close()
		return conn
	}

	// the client goes away before sending meta
	require.NoError(t, dial().Close())
	require.ErrorContains(t, <-errs, "failed to read meta info")

	conn := dial()
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.WriteJSON(StreamMeta{PeerID: srcID, SpanContext: []byte("garbage")}))
	require.ErrorContains(t, <-errs, "failed to unmarshal span context")
	_, _, err := conn.ReadMessage()
	require.Error(t, err, "the server must close the connection")

	_, err = p.NewClientStream(host.StreamInfo{RemotePeerAddress: "127.0.0.1:1"}, t.Context(), srcID, clientTLSConfig)
	require.Error(t, err)
}

func TestExpectedPeerID(t *testing.T) {
	t.Parallel()
	_, err := expectedPeerIDFromRequest(nil)
	require.ErrorContains(t, err, "missing TLS connection state")
	_, err = expectedPeerIDFromRequest(&http.Request{})
	require.ErrorContains(t, err, "missing TLS connection state")
	_, err = expectedPeerIDFromRequest(&http.Request{TLS: &tls.ConnectionState{}})
	require.ErrorContains(t, err, "missing verified TLS peer certificate")

	_, err = peerIDFromCertificate(nil)
	require.ErrorContains(t, err, "nil certificate")

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, priv)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	_, err = peerIDFromCertificate(cert)
	require.ErrorContains(t, err, "unsupported public key type [ed25519.PublicKey]")
}
