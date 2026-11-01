/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gwebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// TestKeepalive_ClosesSilentlyCutConnection runs a multiplexed client and server connection
// through a TCP link that, once cut, stops forwarding without closing anything. With a
// keepalive both sides close within Interval plus Timeout; without one both stay open.
func TestKeepalive_ClosesSilentlyCutConnection(t *testing.T) {
	t.Parallel()
	k := Keepalive{Interval: 100 * time.Millisecond, Timeout: 200 * time.Millisecond}
	for _, tt := range []struct {
		name      string
		keepalive Keepalive
		closes    bool
	}{
		{name: "keepalive", keepalive: k, closes: true},
		{name: "no keepalive", closes: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			serverClosed := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&gwebsocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				newServerConn(conn, "peer", noop.NewTracerProvider().Tracer("test"), newMetrics(&disabled.Provider{}), 2, 0, tt.keepalive, func(s host.P2PStream) { _ = s.Close() }, func() { close(serverClosed) }) //nolint:contextcheck // server streams outlive the upgrade request by design
			}))
			t.Cleanup(srv.Close)
			link := newCuttableLink(t, strings.TrimPrefix(srv.URL, "http://"))

			wsConn, resp, err := gwebsocket.DefaultDialer.Dial("ws://"+link.addr()+"/", nil)
			require.NoError(t, err)
			_ = resp.Body.Close()
			clientClosed := make(chan struct{})
			cc := newClientConn(wsConn, noop.NewTracerProvider().Tracer("test"), newMetrics(&disabled.Provider{}), 2, 0, tt.keepalive, func() { close(clientClosed) })
			t.Cleanup(func() { _ = cc.Kill() })

			// Several pings and pongs cross the healthy link without closing it.
			requireBothOpen(t, clientClosed, serverClosed, 5*k.Interval)

			link.cut()
			if tt.closes {
				bound := k.Interval + k.Timeout + time.Second
				requireSideClosed(t, clientClosed, "client", bound)
				requireSideClosed(t, serverClosed, "server", bound)
			} else {
				requireBothOpen(t, clientClosed, serverClosed, 5*(k.Interval+k.Timeout))
			}
		})
	}
}

func requireBothOpen(t *testing.T, client, server <-chan struct{}, d time.Duration) {
	t.Helper()
	select {
	case <-client:
		t.Fatal("client connection closed")
	case <-server:
		t.Fatal("server connection closed")
	case <-time.After(d):
	}
}

func requireSideClosed(t *testing.T, closed <-chan struct{}, side string, d time.Duration) {
	t.Helper()
	select {
	case <-closed:
	case <-time.After(d):
		t.Fatalf("%s connection still open %v after the cut", side, d)
	}
}

// cuttableLink forwards TCP connections to a backend until cut, after which it stops
// forwarding in both directions without closing anything, so neither side sees a reset.
type cuttableLink struct {
	listener net.Listener
	cutOnce  sync.Once
	cutCh    chan struct{}
}

func newCuttableLink(t *testing.T, backend string) *cuttableLink {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	link := &cuttableLink{listener: l, cutCh: make(chan struct{})}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			b, err := net.Dial("tcp", backend)
			if err != nil {
				_ = c.Close()
				continue
			}
			t.Cleanup(func() { _ = c.Close(); _ = b.Close() })
			go link.pipe(b, c)
			go link.pipe(c, b)
		}
	}()
	return link
}

func (l *cuttableLink) addr() string { return l.listener.Addr().String() }

func (l *cuttableLink) cut() { l.cutOnce.Do(func() { close(l.cutCh) }) }

// pipe copies src to dst until cut, then drops everything it reads without closing dst.
func (l *cuttableLink) pipe(dst, src net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if err != nil {
			return
		}
		select {
		case <-l.cutCh:
			continue
		default:
		}
		if _, err := dst.Write(buf[:n]); err != nil {
			return
		}
	}
}
