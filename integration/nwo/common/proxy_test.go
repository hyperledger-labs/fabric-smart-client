/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"io"
	"maps"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// backend listens on a free port and writes name to every connection it accepts.
func backend(t *testing.T, name string) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
			_, _ = c.Write([]byte(name))
		}
	}()
	return l
}

// echoBackend listens on a free port, writes back what each connection sends, and returns
// its address.
func echoBackend(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { defer func() { _ = c.Close() }(); _, _ = io.Copy(c, c) }()
		}
	}()
	return l.Addr().String()
}

func newTestProxy(t *testing.T, backends ...string) *Proxy {
	t.Helper()
	p, err := StartProxy("127.0.0.1:0", backends...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// dial connects through p and returns the name of the backend that answered.
func dial(t *testing.T, p *Proxy) (net.Conn, string) {
	t.Helper()
	c, err := net.Dial("tcp", p.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, 1)
	_, err = c.Read(buf)
	require.NoError(t, err)
	return c, string(buf)
}

func TestProxy_RoundRobin(t *testing.T) {
	t.Parallel()
	p := newTestProxy(t, backend(t, "a").Addr().String(), backend(t, "b").Addr().String())

	got := map[string]int{}
	for range 3 {
		_, name := dial(t, p)
		got[name]++
	}
	require.Equal(t, map[string]int{"a": 2, "b": 1}, got)
}

func TestProxy_SkipsDeadBackend(t *testing.T) {
	t.Parallel()
	dead := backend(t, "a")
	require.NoError(t, dead.Close())
	p := newTestProxy(t, dead.Addr().String(), backend(t, "b").Addr().String())

	for range 3 {
		_, name := dial(t, p)
		require.Equal(t, "b", name)
	}
}

func TestProxy_ConnectionsCountsOpen(t *testing.T) {
	t.Parallel()
	a, b := backend(t, "a").Addr().String(), backend(t, "b").Addr().String()
	p := newTestProxy(t, a, b)

	first, _ := dial(t, p)
	dial(t, p)
	dial(t, p)
	require.Equal(t, map[string]int{a: 2, b: 1}, p.Connections())

	require.NoError(t, first.Close())
	require.Eventually(t, func() bool {
		return maps.Equal(map[string]int{a: 1, b: 1}, p.Connections())
	}, 5*time.Second, 10*time.Millisecond)
}

func TestProxy_CloseEndsPartition(t *testing.T) {
	t.Parallel()
	p := newTestProxy(t, echoBackend(t))
	c, err := net.Dial("tcp", p.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, 1)
	// A round trip first, so the proxy has accepted the connection before Close.
	_, err = c.Write([]byte{'a'})
	require.NoError(t, err)
	_, err = c.Read(buf)
	require.NoError(t, err)

	p.Partition()
	_, err = c.Write([]byte{'b'})
	require.NoError(t, err)
	require.NoError(t, p.Close())

	// Close ended the partition, so the held byte reaches the backend and comes back.
	_, err = c.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "b", string(buf))
}

func TestProxy_PartitionHoldsBytesUntilHeal(t *testing.T) {
	t.Parallel()
	p := newTestProxy(t, echoBackend(t))
	c, err := net.Dial("tcp", p.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	buf := make([]byte, 1)
	roundTrip := func(b byte) error {
		if _, err := c.Write([]byte{b}); err != nil {
			return err
		}
		_, err := c.Read(buf)
		return err
	}
	require.NoError(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, roundTrip('a'))

	p.Partition()
	p.Partition()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(200*time.Millisecond)))
	var nerr net.Error
	require.ErrorAs(t, roundTrip('b'), &nerr)
	require.True(t, nerr.Timeout(), "the connection must stay open while partitioned")
	// A connection accepted during the partition is held too.
	late, err := net.Dial("tcp", p.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = late.Close() })
	_, err = late.Write([]byte{'c'})
	require.NoError(t, err)

	p.Heal()
	p.Heal()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = c.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "b", string(buf))
	require.NoError(t, late.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = late.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "c", string(buf))
}

func TestProxy_SetLatencyDelaysEachChunk(t *testing.T) {
	t.Parallel()
	p := newTestProxy(t, echoBackend(t))
	c, err := net.Dial("tcp", p.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, 1)
	roundTrip := func() time.Duration {
		start := time.Now()
		_, err := c.Write([]byte{'a'})
		require.NoError(t, err)
		_, err = c.Read(buf)
		require.NoError(t, err)
		return time.Since(start)
	}
	require.Less(t, roundTrip(), 200*time.Millisecond)

	const latency = 200 * time.Millisecond
	p.SetLatency(latency)
	// The request and the echo are each delayed.
	require.GreaterOrEqual(t, roundTrip(), 2*latency)

	p.SetLatency(0)
	require.Less(t, roundTrip(), 200*time.Millisecond)
}

// CloseConnections closes both sides of every open connection, as a load balancer that drops
// its connections does, and keeps accepting new ones.
func TestProxy_CloseConnectionsKeepsAccepting(t *testing.T) {
	t.Parallel()
	addr := echoBackend(t)
	p := newTestProxy(t, addr)

	c, err := net.Dial("tcp", p.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, 1)
	// A round trip first, so the proxy has forwarded the connection before CloseConnections.
	_, err = c.Write([]byte{'a'})
	require.NoError(t, err)
	_, err = c.Read(buf)
	require.NoError(t, err)
	require.Equal(t, map[string]int{addr: 1}, p.Connections())

	p.CloseConnections()

	// The client reads the end of the stream, not a timeout: the proxy closed its side.
	_, err = c.Read(buf)
	require.ErrorIs(t, err, io.EOF)
	require.Empty(t, p.Connections(), "a closed connection is no longer counted")

	// The proxy still accepts and forwards a new connection.
	late, err := net.Dial("tcp", p.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = late.Close() })
	require.NoError(t, late.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = late.Write([]byte{'b'})
	require.NoError(t, err)
	_, err = late.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "b", string(buf))
}

// CloseConnections on a partitioned proxy ends the connections it closes at once, while a
// connection accepted afterwards is held until Heal, so a client that redials hangs.
func TestProxy_CloseConnectionsWhilePartitioned(t *testing.T) {
	t.Parallel()
	p := newTestProxy(t, echoBackend(t))
	c, err := net.Dial("tcp", p.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, 1)
	_, err = c.Write([]byte{'a'})
	require.NoError(t, err)
	_, err = c.Read(buf)
	require.NoError(t, err)

	p.Partition()
	closed := make(chan struct{})
	go func() { p.CloseConnections(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "CloseConnections did not return on a partitioned proxy")
	}
	_, err = c.Read(buf)
	require.ErrorIs(t, err, io.EOF)
	require.Empty(t, p.Connections())

	late, err := net.Dial("tcp", p.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = late.Close() })
	_, err = late.Write([]byte{'b'})
	require.NoError(t, err)
	require.NoError(t, late.SetReadDeadline(time.Now().Add(200*time.Millisecond)))
	var nerr net.Error
	_, err = late.Read(buf)
	require.ErrorAs(t, err, &nerr)
	require.True(t, nerr.Timeout(), "a connection accepted while partitioned is held")

	p.Heal()
	require.NoError(t, late.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = late.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "b", string(buf))
}

// CloseConnections on a partitioned proxy returns even when both directions of a connection
// already read a chunk during the partition and hold it for Heal.
func TestProxy_CloseConnectionsWhilePartitionedHoldsBothDirections(t *testing.T) {
	t.Parallel()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	send := make(chan struct{})
	go func() {
		b, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = b.Close() }()
		<-send
		_, _ = b.Write([]byte{'x'})
		_, _ = io.Copy(io.Discard, b)
	}()
	p := newTestProxy(t, l.Addr().String())
	c, err := net.Dial("tcp", p.Addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.Eventually(t, func() bool { return len(p.Connections()) == 1 }, 5*time.Second, 10*time.Millisecond)

	p.Partition()
	t.Cleanup(p.Heal)
	_, err = c.Write([]byte{'a'})
	require.NoError(t, err)
	close(send)
	// No signal shows that a pipe holds its chunk; give both time to read one.
	time.Sleep(200 * time.Millisecond)

	closed := make(chan struct{})
	go func() { p.CloseConnections(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "CloseConnections did not return while both directions held a chunk")
	}
	require.Empty(t, p.Connections())
}

// Concurrent CloseConnections calls on a partitioned proxy close each connection once and
// all return.
func TestProxy_CloseConnectionsConcurrently(t *testing.T) {
	t.Parallel()
	p := newTestProxy(t, echoBackend(t))
	p.Partition()
	t.Cleanup(p.Heal)
	for range 100 {
		c, err := net.Dial("tcp", p.Addr())
		require.NoError(t, err)
		require.Eventually(t, func() bool { return len(p.Connections()) == 1 }, 5*time.Second, time.Millisecond)
		var wg sync.WaitGroup
		wg.Go(p.CloseConnections)
		wg.Go(p.CloseConnections)
		wg.Wait()
		require.Empty(t, p.Connections())
		_ = c.Close()
	}
}
