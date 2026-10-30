/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// After Retire, the next stream to the peer dials a new websocket, which a load balancer can
// route to another replica, while a stream already open on the retired websocket keeps working.
// KillAll still closes the retired websocket.
func TestRetireDialsNewWebsocket(t *testing.T) { //nolint:paralleltest // shares the server-side provider
	testSetup(t)
	serverSide := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	t.Cleanup(func() { _ = serverSide.KillAll() })
	serverTLSConfig, clientTLSConfig, srcID := testMutualTLSConfigs(t, false)

	var websockets atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		websockets.Add(1)
		assert.NoError(t, serverSide.NewServerStream(w, r, func(s host.P2PStream) { //nolint:contextcheck // server streams outlive the upgrade request by design
			go func() {
				for {
					msg, err := readMsg(s)
					if err != nil {
						return
					}
					if sendMsg(s, msg) != nil {
						return
					}
				}
			}()
		}))
	}))
	srv.TLS = serverTLSConfig
	srv.StartTLS()
	t.Cleanup(srv.Close)

	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	peerID := serverPeerID(t, serverTLSConfig)
	open := func(sessionID string) host.P2PStream {
		info := host.StreamInfo{
			RemotePeerID:      peerID,
			RemotePeerAddress: strings.TrimPrefix(srv.URL, "https://"),
			ContextID:         "ctx",
			SessionID:         sessionID,
		}
		s, err := p.NewClientStream(info, context.Background(), srcID, clientTLSConfig)
		require.NoError(t, err)
		return s
	}
	echo := func(s host.P2PStream, msg string) {
		require.NoError(t, sendMsg(s, []byte(msg)))
		got, err := readMsg(s)
		require.NoError(t, err)
		require.Equal(t, msg, string(got))
	}

	old := open("s1")
	echo(old, "one")
	echo(open("s2"), "two")
	require.Equal(t, int32(1), websockets.Load(), "streams to one peer share a websocket")

	p.Retire(peerID)
	fresh := open("s3")
	echo(fresh, "three")
	require.Equal(t, int32(2), websockets.Load(), "a stream after Retire dials again")
	echo(old, "still open")

	require.NoError(t, p.KillAll())
	p.mu.RLock()
	defer p.mu.RUnlock()
	require.Empty(t, p.retired)
	_, err := readMsg(old)
	require.Error(t, err, "KillAll closes the retired websocket")
}
