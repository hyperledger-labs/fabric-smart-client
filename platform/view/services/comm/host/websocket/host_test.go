/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket_test

import (
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/ws"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

type failingStreamProvider struct{ noopStreamProvider }

func (*failingStreamProvider) NewClientStream(host2.StreamInfo, context.Context, host2.PeerID, *tls.Config) (host2.P2PStream, error) {
	return nil, errors.New("dial refused")
}

func TestHostLookupAndNewStream(t *testing.T) {
	t.Parallel()
	router := routing.StaticIDRouter{"peer": {"127.0.0.1:1", "127.0.0.1:2"}}
	h, err := websocket.NewHost("self",
		routing.NewServiceDiscovery(router, routing.AlwaysFirst[host2.PeerIPAddress]()),
		&failingStreamProvider{}, &mockConfig{}, nil)
	require.NoError(t, err)
	h.Wait()

	addrs, ok := h.Lookup("peer")
	require.True(t, ok)
	require.Equal(t, []host2.PeerIPAddress{"127.0.0.1:1", "127.0.0.1:2"}, addrs)
	_, ok = h.Lookup("unknown")
	require.False(t, ok)

	_, err = h.NewStream(t.Context(), host2.StreamInfo{RemotePeerID: "unknown"})
	require.ErrorContains(t, err, "no address found for peer [unknown]")

	_, err = h.NewStream(t.Context(), host2.StreamInfo{RemotePeerID: "peer"})
	require.ErrorContains(t, err, "dial refused")
}

func TestGetNewHostRequiresMutualTLS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	certPEM, keyPEM, err := websocket.GenerateTestCert("node")
	require.NoError(t, err)
	certFile, keyFile := filepath.Join(dir, "n.crt"), filepath.Join(dir, "n.key")
	require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))

	newProvider := func(cfg websocket.Config) interface{ GetNewHost() (host2.P2PHost, error) } {
		es := &mockEndpointService{}
		discovery := routing.NewServiceDiscovery(routing.NewEndpointServiceIDRouter(es), routing.AlwaysFirst[host2.PeerIPAddress]())
		sp := ws.NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
		t.Cleanup(func() { _ = sp.Close() })
		return websocket.NewEndpointBasedProvider(cfg, es, discovery, sp)
	}

	_, err = newProvider(&mockConfig{certPath: certFile}).GetNewHost()
	require.ErrorContains(t, err, "requires TLS and mutual TLS configuration")

	cfg, err := websocket.NewConfigFromProperties("127.0.0.1:0", keyFile, certFile, nil, nil, false, 0, nil)
	require.NoError(t, err)
	_, err = newProvider(cfg).GetNewHost()
	require.ErrorContains(t, err, "requires mutual TLS (client certificates)")

	_, err = newProvider(&mockConfig{certPath: filepath.Join(dir, "missing.crt")}).GetNewHost()
	require.ErrorContains(t, err, "failed to load identity")
}
