/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"maps"
	"net"
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
