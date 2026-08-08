/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"context"
	"strings"
	"testing"
	"time"

	gwebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// echoPingPong reads "ping" messages off the stream and answers "pong", until the stream
// errors out (e.g. because it was closed).
func echoPingPong(t *testing.T, s host.P2PStream) {
	t.Helper()
	go func() {
		for {
			msg, err := readMsg(s)
			if err != nil {
				return
			}
			assert.Equal(t, []byte("ping"), msg)
			if err := sendMsg(s, []byte("pong")); err != nil {
				return
			}
		}
	}()
}

func firstClientConn(p *MultiplexedProvider) *multiplexedClientConn {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, c := range p.clients {
		return c
	}
	return nil
}

func subConnCount(c *multiplexedClientConn) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.subConns)
}

// isClosedChan reports whether ch has been closed (a closed channel yields the zero value with
// ok=false immediately; a non-empty or open channel does not).
func isClosedChan[T any](ch <-chan T) bool {
	select {
	case _, ok := <-ch:
		return !ok
	default:
		return false
	}
}

// requireSubConnResourcesReleased asserts that a closed subConn's own resources - not just its
// entry in the parent's subConns map - have actually been released: its done and receiverChan
// channels are closed, so nothing can be blocked sending on them or select-ing on them forever.
func requireSubConnResourcesReleased(t *testing.T, sc *subConn) {
	t.Helper()
	require.True(t, isClosedChan(sc.done), "subConn.done must be closed")
	require.True(t, isClosedChan(sc.receiverChan), "subConn.receiverChan must be closed")
}

// requirePhysicalConnClosed asserts that the physical websocket/TCP connection underlying a
// multiplexedClientConn has actually been closed (not just logically "killed"), by attempting a
// write and requiring it to fail.
func requirePhysicalConnClosed(t *testing.T, conn *multiplexedClientConn) {
	t.Helper()
	err := conn.write(MultiplexedMessage{ID: "probe-after-close", Msg: []byte("x")})
	require.Error(t, err, "physical connection must be closed")
}

// TestMultiplexedClientSubConnSurvivesCallerContextCancellation is a regression test for the
// pingpong integration test failure: both initiator.go's ping() and responder.go's pong() wrap
// each protocol round in its own context.WithTimeout, cancelled via `defer cancel()` as soon as
// that round's RunView returns. Since the client sub-connection is cached and reused across
// rounds - including when the responder itself needs to open a fresh outgoing stream to reply
// (e.g. after a cache miss in sendTo) - cancelling that per-call context must NOT tear down the
// sub-connection, from either call site. Otherwise round 2 fails as soon as round 1's context is
// cancelled. This verifies both halves of the fix in newClientSubConn:
//  1. the underlying stream's own context is decoupled from the caller's context (so cancelling
//     the caller's context does not close the stream), and
//  2. once the stream IS closed through a legitimate path (explicit Close()), everything is
//     cleaned up with no leaked goroutines or dangling subConns map entries.
func TestMultiplexedClientSubConnSurvivesCallerContextCancellation(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	t.Cleanup(func() {
		goleak.VerifyNone(t, goleak.IgnoreCurrent())
	})

	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	serverTLSConfig, clientTLSConfig, srcID := testMutualTLSConfigs(t, false)

	srv := startTestServer(t, p, serverTLSConfig, func(s host.P2PStream) {
		echoPingPong(t, s)
	})
	t.Cleanup(srv.Close)

	srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")
	info := host.StreamInfo{
		RemotePeerID:      "serverID",
		RemotePeerAddress: srvEndpoint,
		ContextID:         "detach-ctx",
		SessionID:         "detach-sess",
	}

	// Mimic a single protocol round: a short-lived, cancellable context passed to NewClientStream.
	ctx, cancel := context.WithCancel(context.Background())
	client, err := p.NewClientStream(info, ctx, srcID, clientTLSConfig)
	require.NoError(t, err)
	st, ok := client.(*stream)
	require.True(t, ok)
	sc, ok := st.conn.(*subConn)
	require.True(t, ok)

	// Sanity check: the sub-connection is live.
	require.NoError(t, sendMsg(client, []byte("ping")))
	answer, err := readMsg(client)
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), answer)

	conn := firstClientConn(p)
	require.NotNil(t, conn)
	require.Equal(t, 1, subConnCount(conn))

	// Cancel the caller's context, exactly as `ping()`'s and `pong()`'s `defer cancel()` do at
	// the end of each round in integration/fsc/pingpong/initiator.go and responder.go.
	cancel()
	time.Sleep(snoozeTime)

	// The stream must not have picked up the cancellation of the (now unrelated) caller context.
	require.NoError(t, st.ctx.Err(), "stream context must be decoupled from the caller's context")
	require.False(t, isClosedChan(sc.done), "subConn resources must not be released by caller context cancellation")
	require.False(t, isClosedChan(sc.receiverChan), "subConn resources must not be released by caller context cancellation")

	// The sub-connection must still be tracked and fully usable for a subsequent round.
	require.Equal(t, 1, subConnCount(conn))
	require.NoError(t, sendMsg(client, []byte("ping")))
	answer, err = readMsg(client)
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), answer)

	// Now close the stream through a legitimate path and verify full cleanup: no leaked
	// goroutines (asserted in t.Cleanup), the subConn entry is removed, and the subConn's own
	// resources (its channels) are released.
	require.NoError(t, client.Close())
	require.Equal(t, 0, subConnCount(conn))
	requireSubConnResourcesReleased(t, sc)

	// The physical websocket/TCP connection is shared by all sub-connections, so closing just
	// this one must NOT close it - only KillAll (or the peer/connection erroring out) may.
	require.NoError(t, conn.write(MultiplexedMessage{ID: "still-alive-probe", Msg: []byte("x")}),
		"physical connection must stay open after a single sub-connection closes")

	require.NoError(t, p.KillAll())
	p.mu.RLock()
	require.Empty(t, p.clients)
	p.mu.RUnlock()
	requirePhysicalConnClosed(t, conn)
}

// TestMultiplexedClientSubConnCleanupViaKillAllAfterCallerContextCancellation verifies the other
// legitimate cleanup path: top-down teardown via MultiplexedProvider.KillAll() (e.g. when the
// P2P host is stopped), rather than an explicit per-stream Close(). Since the per-call context
// is no longer wired to the stream's lifetime, cancelling it must have no effect on cleanup -
// only KillAll (or the underlying connection erroring out) may close the sub-connection - and
// KillAll must still fully release the sub-connection with no leaked goroutines.
func TestMultiplexedClientSubConnCleanupViaKillAllAfterCallerContextCancellation(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	t.Cleanup(func() {
		goleak.VerifyNone(t, goleak.IgnoreCurrent())
	})

	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	serverTLSConfig, clientTLSConfig, srcID := testMutualTLSConfigs(t, false)

	srv := startTestServer(t, p, serverTLSConfig, func(s host.P2PStream) {
		echoPingPong(t, s)
	})
	t.Cleanup(srv.Close)

	srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")
	info := host.StreamInfo{
		RemotePeerID:      "serverID",
		RemotePeerAddress: srvEndpoint,
		ContextID:         "detach-killall-ctx",
		SessionID:         "detach-killall-sess",
	}

	ctx, cancel := context.WithCancel(context.Background())
	client, err := p.NewClientStream(info, ctx, srcID, clientTLSConfig)
	require.NoError(t, err)
	st, ok := client.(*stream)
	require.True(t, ok)
	sc, ok := st.conn.(*subConn)
	require.True(t, ok)

	require.NoError(t, sendMsg(client, []byte("ping")))
	_, err = readMsg(client)
	require.NoError(t, err)

	conn := firstClientConn(p)
	require.NotNil(t, conn)
	require.Equal(t, 1, subConnCount(conn))

	// Cancel the caller context well before any explicit Close() - this must be a no-op as far
	// as the sub-connection's lifetime and resources are concerned.
	cancel()
	time.Sleep(snoozeTime)
	require.Equal(t, 1, subConnCount(conn), "sub-connection must survive caller context cancellation")
	require.False(t, isClosedChan(sc.done), "subConn resources must not be released by caller context cancellation")
	require.False(t, isClosedChan(sc.receiverChan), "subConn resources must not be released by caller context cancellation")

	// Tear everything down top-down, without ever calling client.Close() directly.
	require.NoError(t, p.KillAll())

	p.mu.RLock()
	require.Empty(t, p.clients)
	p.mu.RUnlock()
	require.Equal(t, 0, subConnCount(conn))
	requireSubConnResourcesReleased(t, sc)
	requirePhysicalConnClosed(t, conn)
}

// TestSimpleProviderClientStreamSurvivesCallerContextCancellation is the SimpleProvider analogue
// of TestMultiplexedClientSubConnSurvivesCallerContextCancellation: NewClientStream must decouple
// the returned stream's lifetime from the caller-supplied context, and closing the stream through
// its legitimate Close() path must leave no leaked goroutines behind.
func TestSimpleProviderClientStreamSurvivesCallerContextCancellation(t *testing.T) { //nolint:paralleltest
	testSetup(t)
	t.Cleanup(func() {
		goleak.VerifyNone(t, goleak.IgnoreCurrent())
	})

	p := NewSimpleProvider()
	serverTLSConfig, clientTLSConfig, srcID := testMutualTLSConfigs(t, false)

	srv := startTestServer(t, p, serverTLSConfig, func(s host.P2PStream) {
		echoPingPong(t, s)
	})
	t.Cleanup(srv.Close)

	srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")
	info := host.StreamInfo{
		RemotePeerID:      "serverID",
		RemotePeerAddress: srvEndpoint,
		ContextID:         "simple-detach-ctx",
		SessionID:         "simple-detach-sess",
	}

	ctx, cancel := context.WithCancel(context.Background())
	client, err := p.NewClientStream(info, ctx, srcID, clientTLSConfig)
	require.NoError(t, err)

	require.NoError(t, sendMsg(client, []byte("ping")))
	answer, err := readMsg(client)
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), answer)

	cancel()
	time.Sleep(snoozeTime)

	st, ok := client.(*stream)
	require.True(t, ok)
	require.NoError(t, st.ctx.Err(), "stream context must be decoupled from the caller's context")

	require.NoError(t, sendMsg(client, []byte("ping")))
	answer, err = readMsg(client)
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), answer)

	require.NoError(t, client.Close())

	// The underlying physical websocket connection (owned exclusively by this single stream,
	// unlike the multiplexed case) must actually be closed too, not just logically detached.
	writeErr := st.conn.(*gwebsocket.Conn).WriteMessage(gwebsocket.BinaryMessage, []byte("probe-after-close"))
	require.Error(t, writeErr, "underlying websocket connection must be closed")
}
