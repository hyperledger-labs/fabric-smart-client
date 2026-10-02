/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gwebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// rawPeer drives one side of a multiplexed connection frame by frame, so a test can send
// frames the real peer never produces. Incoming frames land on frames; sync blocks until the
// other side has processed every frame written before it.
type rawPeer struct {
	t      *testing.T
	conn   *gwebsocket.Conn
	frames chan MultiplexedMessage
	pongs  chan struct{}
	done   chan struct{}
}

func newRawPeer(t *testing.T, conn *gwebsocket.Conn) *rawPeer {
	t.Helper()
	r := &rawPeer{
		t:      t,
		conn:   conn,
		frames: make(chan MultiplexedMessage, 1000),
		pongs:  make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
	conn.SetPongHandler(func(string) error {
		r.pongs <- struct{}{}
		return nil
	})
	go func() {
		defer close(r.done)
		for {
			var mm MultiplexedMessage
			if err := conn.ReadJSON(&mm); err != nil {
				return
			}
			r.frames <- mm
		}
	}()
	t.Cleanup(func() { _ = conn.Close() })
	return r
}

// dialRaw opens a raw websocket to a MultiplexedProvider server.
func dialRaw(t *testing.T, srvURL string, cfg *tls.Config) *rawPeer {
	t.Helper()
	dialer := gwebsocket.Dialer{TLSClientConfig: cfg}
	conn, resp, err := dialer.Dial("wss://"+strings.TrimPrefix(srvURL, "https://")+"/p2p", nil)
	require.NoError(t, err)
	_ = resp.Body.Close()
	return newRawPeer(t, conn)
}

func (r *rawPeer) send(mm MultiplexedMessage) {
	r.t.Helper()
	require.NoError(r.t, r.conn.WriteJSON(mm))
}

func (r *rawPeer) sendMeta(id SubConnId, peerID host.PeerID) {
	r.t.Helper()
	r.send(MultiplexedMessage{ID: id, Msg: mustMarshal(StreamMeta{SessionID: "s" + id, PeerID: peerID})})
}

// sync relies on the other side's reader answering a ping only after it has handled every
// frame that precedes it on the wire.
func (r *rawPeer) sync() {
	r.t.Helper()
	require.NoError(r.t, r.conn.WriteControl(gwebsocket.PingMessage, nil, time.Now().Add(time.Second)))
	select {
	case <-r.pongs:
	case <-r.done:
		r.t.Fatal("connection closed while waiting for pong")
	case <-time.After(5 * time.Second):
		r.t.Fatal("timeout waiting for pong")
	}
}

// startMultiplexedServer starts a MultiplexedProvider server whose accepted streams are
// published on the returned channel, and returns the client TLS config and peer ID to dial it.
func startMultiplexedServer(t *testing.T, maxSubConns int) (url string, streams chan host.P2PStream, clientTLS *tls.Config, clientID host.PeerID) {
	t.Helper()
	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, maxSubConns)
	t.Cleanup(func() { _ = p.Close() })
	serverTLS, clientTLS, clientID := testMutualTLSConfigs(t, false)
	streams = make(chan host.P2PStream, 10)
	srv := startTestServer(t, p, serverTLS, func(s host.P2PStream) { streams <- s })
	t.Cleanup(srv.Close)
	return srv.URL, streams, clientTLS, clientID
}

// next returns the next frame the other side sent.
func (r *rawPeer) next() MultiplexedMessage {
	r.t.Helper()
	select {
	case mm := <-r.frames:
		return mm
	case <-time.After(5 * time.Second):
		r.t.Fatal("timeout waiting for frame")
		return MultiplexedMessage{}
	}
}

// fillUntilClosed writes data frames for id until the other side answers with the EOF frame
// that closes it. Frames are sent one at a time with a sync in between, so no data frame for
// id is in flight once the sub-connection is closed.
func (r *rawPeer) fillUntilClosed(id SubConnId) {
	r.t.Helper()
	// receiverChan, the stream's reads channel and the value readMessages holds in hand
	for range 2*100 + 2 {
		r.send(MultiplexedMessage{ID: id, Msg: []byte("x")})
		r.sync()
		select {
		case mm := <-r.frames:
			require.Equal(r.t, MultiplexedMessage{ID: id, Err: "EOF"}, mm)
			return
		default:
		}
	}
	r.t.Fatal("sub-connection was never closed")
}

func serverParent(t *testing.T, s host.P2PStream) *multiplexedBaseConn {
	t.Helper()
	return s.(*stream).conn.(*subConnWithSpan).parentConn
}

func hasSubConn(c *multiplexedBaseConn, id SubConnId) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.subConns[id]
	return ok
}

func closingCount(c *multiplexedBaseConn) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.closing)
}

func receiveStream(t *testing.T, streams <-chan host.P2PStream) host.P2PStream {
	t.Helper()
	select {
	case s := <-streams:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server stream")
		return nil
	}
}

// The server-side cap protects a node from a peer that bypasses the client-side check.
func TestServerRejectsSubConnsBeyondMax(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	url, streams, clientTLS, clientID := startMultiplexedServer(t, 1)
	raw := dialRaw(t, url, clientTLS)

	raw.sendMeta("1", clientID)
	s1 := receiveStream(t, streams)
	raw.sendMeta("2", clientID)
	require.Equal(t, MultiplexedMessage{ID: "2", Err: "max sub-connections reached"}, raw.next())

	// a second meta for a live sub-connection is data for it, not a new stream
	raw.sendMeta("1", clientID)
	raw.sync()
	require.Empty(t, streams)

	// Data the client sent on the rejected sub-connection before seeing the rejection is
	// dropped, even once a slot is free again.
	raw.send(MultiplexedMessage{ID: "1", Err: "EOF"})
	require.Equal(t, MultiplexedMessage{ID: "1", Err: "EOF"}, raw.next())
	raw.send(MultiplexedMessage{ID: "2", Msg: []byte("late")})
	raw.sync()
	raw.send(MultiplexedMessage{ID: "2", Err: "EOF"})
	raw.sync()
	require.Equal(t, 0, closingCount(serverParent(t, s1)))
}

// Closing a sub-connection must not turn the frames the client already sent on it into a
// new sub-connection: their payload is not a StreamMeta, so the server would take them for a
// peer ID mismatch and kill every sub-connection on the physical connection.
func TestServerDropsLateFramesForClosedSubConn(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	url, streams, clientTLS, clientID := startMultiplexedServer(t, 0)
	raw := dialRaw(t, url, clientTLS)

	raw.sendMeta("1", clientID)
	s1 := receiveStream(t, streams)
	raw.sendMeta("2", clientID)
	s2 := receiveStream(t, streams)
	parent := serverParent(t, s2)

	require.NoError(t, s1.Close())
	require.Equal(t, MultiplexedMessage{ID: "1", Err: "EOF"}, raw.next())
	raw.send(MultiplexedMessage{ID: "1", Msg: []byte("late")})
	raw.sync()
	require.True(t, hasSubConn(parent, "2"))
	require.Equal(t, 1, closingCount(parent))

	// the client's own Err frame acknowledges the close
	raw.send(MultiplexedMessage{ID: "1", Err: "EOF"})
	raw.sync()
	require.Equal(t, 0, closingCount(parent))

	// a close the client starts is acknowledged, not left waiting for an answer
	raw.send(MultiplexedMessage{ID: "2", Err: "EOF"})
	require.Equal(t, MultiplexedMessage{ID: "2", Err: "EOF"}, raw.next())
	raw.sync()
	require.False(t, hasSubConn(parent, "2"))
	require.Equal(t, 0, closingCount(parent))
}

func TestServerClosesSlowSubConn(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	url, streams, clientTLS, clientID := startMultiplexedServer(t, 0)
	raw := dialRaw(t, url, clientTLS)

	raw.sendMeta("1", clientID)
	slow := receiveStream(t, streams)
	raw.fillUntilClosed("1")
	require.False(t, hasSubConn(serverParent(t, slow), "1"))
	// a sender does not stop the moment the receiver falls behind
	raw.send(MultiplexedMessage{ID: "1", Msg: []byte("x")})
	raw.sync()

	// the physical connection still serves new sub-connections
	raw.sendMeta("2", clientID)
	s2 := receiveStream(t, streams)
	require.True(t, hasSubConn(serverParent(t, s2), "2"))
}

func TestServerIgnoresErrorFrameForUnknownSubConn(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	url, streams, clientTLS, clientID := startMultiplexedServer(t, 0)
	raw := dialRaw(t, url, clientTLS)

	raw.send(MultiplexedMessage{ID: "999", Err: "x"})
	raw.sendMeta("1", clientID)
	receiveStream(t, streams)
}

// startRawServer accepts one plain websocket connection and hands it to the test, so the test
// can play the server side of a MultiplexedProvider client frame by frame.
func startRawServer(t *testing.T) (addr string, peers <-chan *rawPeer) {
	t.Helper()
	ch := make(chan *rawPeer, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&gwebsocket.Upgrader{}).Upgrade(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		ch <- newRawPeer(t, conn)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://"), ch
}

func TestClientSubConnEdgeCases(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	addr, peers := startRawServer(t)
	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	t.Cleanup(func() { _ = p.Close() })
	info := host.StreamInfo{RemotePeerID: "server", RemotePeerAddress: addr, SessionID: "s1"}

	// a stream nobody reads from
	_, err := p.NewClientStream(info, t.Context(), "client", nil)
	require.NoError(t, err)
	raw := <-peers
	require.Equal(t, "1", raw.next().ID)
	conn := firstClientConn(p)
	require.Equal(t, 1, subConnCount(conn))

	// Frames for an unknown ID, with or without an error, are dropped.
	raw.send(MultiplexedMessage{ID: "999", Err: "x"})
	raw.send(MultiplexedMessage{ID: "999", Msg: []byte("x")})
	raw.sync()
	require.Equal(t, 1, subConnCount(conn))

	raw.fillUntilClosed("1")
	require.Equal(t, 0, subConnCount(conn))
	require.Equal(t, 1, closingCount(conn.multiplexedBaseConn))
	raw.send(MultiplexedMessage{ID: "1", Err: "EOF"})
	raw.sync()
	require.Equal(t, 0, closingCount(conn.multiplexedBaseConn))

	// the physical connection still serves new sub-connections
	info.SessionID = "s2"
	_, err = p.NewClientStream(info, t.Context(), "client", nil)
	require.NoError(t, err)
	require.Equal(t, "2", raw.next().ID)
	require.Same(t, conn, firstClientConn(p))
}

func TestMultiplexedProviderClose(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	serverTLS, clientTLS, clientID := testMutualTLSConfigs(t, false)
	srv := startTestServer(t, NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0), serverTLS, func(host.P2PStream) {})
	t.Cleanup(srv.Close)
	info := host.StreamInfo{RemotePeerID: "server", RemotePeerAddress: strings.TrimPrefix(srv.URL, "https://"), SessionID: "s1"}

	// Concurrent stream creation and Close exercise the provider and connection locks.
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Go(func() {
			info := info
			info.SessionID = fmt.Sprintf("s%d", i)
			_, _ = p.NewClientStream(info, t.Context(), clientID, clientTLS)
		})
	}
	_, err := p.NewClientStream(info, t.Context(), clientID, clientTLS)
	require.NoError(t, err)
	conn := firstClientConn(p)
	require.NotNil(t, conn)
	conn.mu.RLock()
	sc := conn.subConns["1"]
	conn.mu.RUnlock()
	require.NotNil(t, sc)
	require.Equal(t, "1", sc.ID())

	require.NoError(t, p.Close())
	require.NoError(t, p.Close())
	wg.Wait()

	require.ErrorIs(t, sc.WriteMessage(gwebsocket.BinaryMessage, []byte("x")), gwebsocket.ErrCloseSent)
	require.False(t, sc.deliver(result{value: []byte("x")}))
	_, _, err = sc.ReadMessage()
	require.True(t, gwebsocket.IsCloseError(err, gwebsocket.CloseAbnormalClosure), "got %v", err)
	requirePhysicalConnClosed(t, conn)

	// Streams created concurrently with Close may outlive it; clean them up.
	p.mu.RLock()
	for _, c := range p.clients {
		_ = c.Kill()
	}
	p.mu.RUnlock()
}

func TestMultiplexedProviderStreamErrors(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	t.Cleanup(func() { _ = p.Close() })

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	_, err = p.NewClientStream(host.StreamInfo{RemotePeerAddress: addr}, t.Context(), "client", &tls.Config{MinVersion: tls.VersionTLS13})
	require.ErrorContains(t, err, "failed to open websocket")
	p.mu.RLock()
	require.Empty(t, p.clients)
	p.mu.RUnlock()

	requireServerStreamErrors(t, p)
}

// requireServerStreamErrors checks that NewServerStream rejects a request without TLS state
// and a TLS request that is not a websocket upgrade.
func requireServerStreamErrors(t *testing.T, p websocket.StreamProvider) {
	t.Helper()
	noCallback := func(host.P2PStream) { t.Error("unexpected stream") }

	err := p.NewServerStream(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/p2p", nil), noCallback)
	require.ErrorContains(t, err, "failed extracting expected peerID")

	_, clientTLS, _ := testMutualTLSConfigs(t, false)
	cert, err := x509.ParseCertificate(clientTLS.Certificates[0].Certificate[0])
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/p2p", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
	err = p.NewServerStream(httptest.NewRecorder(), req, noCallback)
	require.ErrorContains(t, err, "failed to open websocket")
}

// pipeHijacker is an http.ResponseWriter whose Hijack returns one end of a net.Pipe, so
// gorilla/websocket's server-side Upgrade runs without a TCP socket. net.Pipe is unbuffered:
// a Write blocks until the other end reads it, which makes a stuck write deterministic.
type pipeHijacker struct {
	conn net.Conn
	br   *bufio.Reader
}

func (*pipeHijacker) Header() http.Header         { return http.Header{} }
func (*pipeHijacker) Write(p []byte) (int, error) { return len(p), nil }
func (*pipeHijacker) WriteHeader(int)             {}
func (h *pipeHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.conn, bufio.NewReadWriter(h.br, bufio.NewWriter(h.conn)), nil
}

// newPipedServerConn runs a multiplexedServerConn on one end of a net.Pipe and returns the
// raw websocket on the other end, so the test acts as the peer and decides when to read.
func newPipedServerConn(t *testing.T, expectedPeerID host.PeerID, maxSubConns int, cb func(host.P2PStream)) (*multiplexedServerConn, *gwebsocket.Conn) {
	t.Helper()

	serverSide, clientSide := net.Pipe()
	// Unblocks the upgrade goroutine if the helper fails before handing the conns over.
	t.Cleanup(func() {
		_ = serverSide.Close()
		_ = clientSide.Close()
	})

	serverWSCh := make(chan *gwebsocket.Conn, 1)
	serverErrCh := make(chan error, 1)
	go func() {
		br := bufio.NewReader(serverSide)
		req, err := http.ReadRequest(br)
		if err != nil {
			serverErrCh <- err
			return
		}
		conn, err := (&gwebsocket.Upgrader{}).Upgrade(&pipeHijacker{conn: serverSide, br: br}, req, nil)
		if err != nil {
			serverErrCh <- err
			return
		}
		serverWSCh <- conn
	}()

	// pipeHijacker drops the HTTP error a failed Upgrade writes, so bound the handshake.
	dialer := gwebsocket.Dialer{
		NetDialContext: func(context.Context, string, string) (net.Conn, error) {
			return clientSide, nil
		},
		HandshakeTimeout: 2 * time.Second,
	}
	clientWSConn, resp, err := dialer.Dial("ws://pipe/", nil)
	require.NoError(t, err)
	_ = resp.Body.Close()

	var serverWSConn *gwebsocket.Conn
	select {
	case serverWSConn = <-serverWSCh:
	case err := <-serverErrCh:
		t.Fatalf("server-side websocket upgrade failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server-side websocket upgrade")
	}

	sc := newServerConn(serverWSConn, expectedPeerID, noop.NewTracerProvider().Tracer("test"), newMetrics(&disabled.Provider{}), maxSubConns, cb)
	return sc, clientWSConn
}

// TestNewServerSubConn_RejectionWriteDoesNotHoldMapLock checks that newServerSubConn releases
// the sub-connection map lock (mu) before writing a rejection frame. The peer does not read,
// so the write blocks on the pipe while holding writeMu; mu must stay acquirable meanwhile.
func TestNewServerSubConn_RejectionWriteDoesNotHoldMapLock(t *testing.T) { //nolint:paralleltest
	testSetup(t)

	const peerID host.PeerID = "legit-peer"

	tests := []struct {
		name        string
		maxSubConns int
		claimedPeer host.PeerID
		wantErr     string
	}{
		// The established sub-connection fills the cap, so ID 2 is rejected.
		{name: "max sub-connections", maxSubConns: 1, claimedPeer: peerID, wantErr: "max sub-connections reached"},
		{name: "peer identity binding", maxSubConns: 2, claimedPeer: "spoofed-peer", wantErr: "peer identity binding failed"},
	}
	for _, tt := range tests { //nolint:paralleltest // goleak snapshots need serial subtests
		t.Run(tt.name, func(t *testing.T) { //nolint:paralleltest
			ignore := goleak.IgnoreCurrent()
			t.Cleanup(func() { goleak.VerifyNone(t, ignore) })

			streams := make(chan host.P2PStream, 1)
			sc, peer := newPipedServerConn(t, peerID, tt.maxSubConns, func(s host.P2PStream) { streams <- s })
			t.Cleanup(func() { _ = peer.Close() })

			require.NoError(t, peer.WriteJSON(MultiplexedMessage{ID: "1", Msg: mustMarshal(StreamMeta{PeerID: peerID})}))
			select {
			case <-streams:
			case <-time.After(2 * time.Second):
				t.Fatal("server never accepted the first sub-connection")
			}

			require.NoError(t, peer.WriteJSON(MultiplexedMessage{ID: "2", Msg: mustMarshal(StreamMeta{PeerID: tt.claimedPeer})}))

			// The read loop is now stuck writing the rejection frame to a peer that does not read.
			require.Eventually(t, func() bool {
				if sc.writeMu.TryLock() {
					sc.writeMu.Unlock()
					return false
				}
				return true
			}, 2*time.Second, 5*time.Millisecond, "rejection write never blocked")

			require.True(t, sc.mu.TryLock(), "mu is held across the blocked rejection write")
			sc.mu.Unlock()

			var rejection MultiplexedMessage
			require.NoError(t, peer.ReadJSON(&rejection))
			require.Equal(t, "2", rejection.ID)
			require.Equal(t, tt.wantErr, rejection.Err)
		})
	}
}

// TestNewClientStream_HangingPeerDoesNotBlockHealthyPeer checks that an unresponsive peer
// that accepts TCP connections but hangs during the WebSocket handshake does not block stream
// creation to healthy peers, and does not hold the provider lock while dialing (Issue #1975).
func TestNewClientStream_HangingPeerDoesNotBlockHealthyPeer(t *testing.T) { //nolint:paralleltest
	testSetup(t)

	hang, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = hang.Close() }()

	go func() {
		for {
			c, err := hang.Accept()
			if err != nil {
				return
			}
			defer func() { _ = c.Close() }()
		}
	}()

	healthy, _ := startRawServer(t)
	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	t.Cleanup(func() { _ = p.Close() })

	hangErrCh := make(chan error, 1)
	go func() {
		_, err := p.NewClientStream(host.StreamInfo{RemotePeerAddress: hang.Addr().String()}, t.Context(), "client", nil)
		hangErrCh <- err
	}()

	// Allow the hanging dial to begin and hold the in-flight dial entry for hang.
	time.Sleep(100 * time.Millisecond)

	// Verify the provider lock is NOT held while the dial is blocked on the hanging peer.
	require.True(t, p.mu.TryLock(), "provider lock p.mu is held while dial is hanging")
	p.mu.Unlock()

	// Stream creation to the healthy peer must succeed promptly and not be blocked by the hanging dial.
	healthyDone := make(chan error, 1)
	go func() {
		_, err := p.NewClientStream(host.StreamInfo{RemotePeerAddress: healthy}, t.Context(), "client", nil)
		healthyDone <- err
	}()

	select {
	case err := <-healthyDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("healthy peer stream creation was blocked by hanging peer dial")
	}
}

// TestNewClientStream_ConcurrentDialsShareConnection checks that concurrent callers attempting
// to open a stream to the same address share a single physical websocket connection (Issue #1975).
func TestNewClientStream_ConcurrentDialsShareConnection(t *testing.T) { //nolint:paralleltest
	testSetup(t)

	addr, peers := startRawServer(t)
	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	t.Cleanup(func() { _ = p.Close() })

	const concurrentCallers = 10
	var wg sync.WaitGroup
	errCh := make(chan error, concurrentCallers)

	for i := range concurrentCallers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			info := host.StreamInfo{
				RemotePeerAddress: addr,
				RemotePeerID:      "server",
				SessionID:         fmt.Sprintf("session-%d", id),
			}
			_, err := p.NewClientStream(info, t.Context(), "client", nil)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	// Exactly one physical client connection should have been established to the server.
	select {
	case <-peers:
		// First peer received
	default:
		t.Fatal("expected at least one server connection")
	}

	select {
	case <-peers:
		t.Fatal("expected only one physical connection to be dialed, but received multiple")
	default:
		// Expected: no duplicate connection
	}
}
