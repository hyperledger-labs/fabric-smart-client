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
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/goleak"

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
					RemotePeerID:      serverPeerID(t, serverTLSConfig),
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

func TestSimpleProviderStreamErrors(t *testing.T) {
	t.Parallel()

	testSetup(t)
	p := NewSimpleProvider()
	t.Cleanup(func() { _ = p.Close() })
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

	_, err := p.NewClientStream(host.StreamInfo{RemotePeerAddress: "127.0.0.1:1"}, t.Context(), srcID, clientTLSConfig)
	require.Error(t, err)
}

// Both providers accept a stream whose span context does not decode and give it an empty
// parent span: tracing is best effort and must not decide whether a peer can connect.
func TestServerAcceptsMalformedSpanContext(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	meta := func(peerID host.PeerID) StreamMeta {
		return StreamMeta{PeerID: peerID, SpanContext: []byte("garbage")}
	}
	requireAccepted := func(t *testing.T, s host.P2PStream, peerID host.PeerID) {
		t.Helper()
		require.Equal(t, peerID, s.RemotePeerID())
		// The multiplexed stream carries the span its provider starts; the noop tracer of
		// these tests passes the (empty) remote parent through unchanged.
		require.False(t, trace.SpanContextFromContext(s.Context()).IsValid())
	}

	t.Run("simple", func(t *testing.T) { //nolint:paralleltest
		serverTLS, clientTLS, clientID := testMutualTLSConfigs(t, false)
		streams := make(chan host.P2PStream, 1)
		srv := startTestServer(t, NewSimpleProvider(), serverTLS, func(s host.P2PStream) { streams <- s })
		t.Cleanup(srv.Close)
		conn, resp, err := (&websocket.Dialer{TLSClientConfig: clientTLS}).Dial("wss://"+strings.TrimPrefix(srv.URL, "https://")+"/p2p", nil)
		require.NoError(t, err)
		_ = resp.Body.Close()
		t.Cleanup(func() { _ = conn.Close() })

		require.NoError(t, conn.WriteJSON(meta(clientID)))
		requireAccepted(t, receiveStream(t, streams), clientID)
	})

	t.Run("multiplexed", func(t *testing.T) { //nolint:paralleltest
		url, streams, clientTLS, clientID := startMultiplexedServer(t, 0)
		raw := dialRaw(t, url, clientTLS)

		raw.send(MultiplexedMessage{ID: "1", Msg: mustMarshal(meta(clientID))})
		requireAccepted(t, receiveStream(t, streams), clientID)
	})
}

// Each stream owns its websocket, and http.Server.Shutdown leaves hijacked connections open,
// so the provider must end the streams it opened and accepted itself.
func TestSimpleProviderClose(t *testing.T) { //nolint:paralleltest // goleak checks every goroutine in the process
	testSetup(t)
	ignore := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, ignore) })

	p := NewSimpleProvider()
	serverTLS, clientTLS, clientID := testMutualTLSConfigs(t, false)
	streams := make(chan host.P2PStream, 10)
	onStream := func(s host.P2PStream) { streams <- s }
	errs := make(chan error, 10)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errs <- p.NewServerStream(w, r, onStream) //nolint:contextcheck // server streams outlive the upgrade request by design
	}))
	srv.TLS = serverTLS
	srv.StartTLS()
	t.Cleanup(srv.Close)
	info := host.StreamInfo{RemotePeerID: serverPeerID(t, serverTLS), RemotePeerAddress: strings.TrimPrefix(srv.URL, "https://"), SessionID: "s1"}
	open := func(p *SimpleProvider) host.P2PStream {
		t.Helper()
		c, err := p.NewClientStream(info, t.Context(), clientID, clientTLS)
		require.NoError(t, err)
		return c
	}

	// Closed streams leave the provider, on both ends of the websocket.
	require.NoError(t, open(p).Close())
	require.NoError(t, <-errs)
	receiveStream(t, streams)
	require.Eventually(t, func() bool { return trackedStreams(p) == 0 }, 5*time.Second, 10*time.Millisecond)

	client := open(p)
	require.NoError(t, <-errs)
	server := receiveStream(t, streams)
	require.Equal(t, 2, trackedStreams(p))

	require.NoError(t, p.Close())
	require.NoError(t, p.Close())
	_, err := client.Read(make([]byte, 1))
	require.Error(t, err)
	_, err = server.Read(make([]byte, 1))
	require.Error(t, err)
	_, err = p.NewClientStream(info, t.Context(), clientID, clientTLS)
	require.ErrorContains(t, err, "provider is closed")

	// The closed provider rejects new websockets.
	other := NewSimpleProvider()
	t.Cleanup(func() { _ = other.Close() })
	late := open(other)
	require.ErrorContains(t, <-errs, "provider is closed")
	_, err = late.Read(make([]byte, 1))
	require.Error(t, err)
	require.Zero(t, trackedStreams(p))
}

func trackedStreams(p *SimpleProvider) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.streams)
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
