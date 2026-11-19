/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	commhost "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/routing"
)

// TestHostStartFailsOnInvalidAddress checks that Start reports a listen address that cannot
// be bound, without leaking a goroutine.
func TestHostStartFailsOnInvalidAddress(t *testing.T) { //nolint:paralleltest
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	host, err := websocket.NewHost(
		"test-node",
		routing.NewServiceDiscovery(&routing.StaticIDRouter{}, routing.AlwaysFirst[commhost.PeerIPAddress]()),
		noopProvider(),
		&mockConfig{listenAddress: "invalid:address"},
		nil, // caPoolProvider
	)
	require.NoError(t, err)

	require.ErrorContains(t, host.Start(func(commhost.P2PStream) {}), "failed to listen")
}

// TestHostStartAcceptsConnectionsAtOnce checks that the listen address accepts a
// connection as soon as Start returns.
func TestHostStartAcceptsConnectionsAtOnce(t *testing.T) { //nolint:paralleltest
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	h, err := websocket.NewHost(
		"test-node",
		routing.NewServiceDiscovery(&routing.StaticIDRouter{}, routing.AlwaysFirst[commhost.PeerIPAddress]()),
		noopProvider(),
		&mockConfig{listenAddress: "127.0.0.1:0"},
		nil, // caPoolProvider
	)
	require.NoError(t, err)
	defer func() { _ = h.Close() }()

	require.NoError(t, h.Start(func(commhost.P2PStream) {}))

	conn, err := net.DialTimeout("tcp", h.Addr(), time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

// TestHostStartFailsOnBoundAddress checks that Start reports an address that another
// listener already holds.
func TestHostStartFailsOnBoundAddress(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	h, err := websocket.NewHost(
		"test-node",
		routing.NewServiceDiscovery(&routing.StaticIDRouter{}, routing.AlwaysFirst[commhost.PeerIPAddress]()),
		noopProvider(),
		&mockConfig{listenAddress: l.Addr().String()},
		nil, // caPoolProvider
	)
	require.NoError(t, err)

	require.ErrorContains(t, h.Start(func(commhost.P2PStream) {}), "failed to listen")
}

// TestNewHostPropagatesClientTLSConfigError checks that a client TLS config error is
// returned instead of panicking.
func TestNewHostPropagatesClientTLSConfigError(t *testing.T) {
	t.Parallel()
	_, err := websocket.NewHost(
		"test-node",
		routing.NewServiceDiscovery(&routing.StaticIDRouter{}, routing.AlwaysFirst[commhost.PeerIPAddress]()),
		noopProvider(),
		&mockConfig{clientErr: errors.New("boom")},
		nil,
	)
	require.ErrorContains(t, err, "failed to build client TLS config")
}

// TestNewHostPropagatesServerTLSConfigError checks that a server TLS config error is
// returned instead of panicking.
func TestNewHostPropagatesServerTLSConfigError(t *testing.T) {
	t.Parallel()
	_, err := websocket.NewHost(
		"test-node",
		routing.NewServiceDiscovery(&routing.StaticIDRouter{}, routing.AlwaysFirst[commhost.PeerIPAddress]()),
		noopProvider(),
		&mockConfig{serverErr: errors.New("boom")},
		nil,
	)
	require.ErrorContains(t, err, "failed to build server TLS config")
}

// mockConfig implements websocket.Config for testing
type mockConfig struct {
	listenAddress        commhost.PeerIPAddress
	certPath             string
	clientErr, serverErr error
}

func (m *mockConfig) ListenAddress() commhost.PeerIPAddress { return m.listenAddress }
func (m *mockConfig) ClientTLSConfig() (*tls.Config, error) {
	return nil, m.clientErr
}

func (m *mockConfig) ServerTLSConfig(websocket.ExtraCAPoolProvider) (*tls.Config, error) {
	return nil, m.serverErr
}
func (m *mockConfig) CertPath() string               { return m.certPath }
func (*mockConfig) MaxSubConns() int                 { return 100 }
func (*mockConfig) MaxRecvMsgSize() int              { return 10 * 1024 * 1024 }
func (*mockConfig) MaxSendMsgSize() int              { return 10 * 1024 * 1024 }
func (*mockConfig) ReadHeaderTimeout() time.Duration { return 10 * time.Second }

func (*mockConfig) ReadTimeout() time.Duration { return 30 * time.Second }

func (*mockConfig) WriteTimeout() time.Duration { return 30 * time.Second }

func (*mockConfig) IdleTimeout() time.Duration       { return 120 * time.Second }
func (*mockConfig) CORSAllowedOrigins() []string     { return nil }
func (*mockConfig) KeepaliveInterval() time.Duration { return 0 }
func (*mockConfig) KeepaliveTimeout() time.Duration  { return 0 }

// noopProvider returns a stream provider that does nothing
func noopProvider() websocket.StreamProvider {
	return &noopStreamProvider{}
}

type noopStreamProvider struct{}

func (*noopStreamProvider) NewClientStream(_ commhost.StreamInfo, _ context.Context, _ commhost.PeerID, _ *tls.Config) (commhost.P2PStream, error) {
	return nil, nil
}

func (*noopStreamProvider) NewServerStream(_ http.ResponseWriter, _ *http.Request, _ func(commhost.P2PStream)) error {
	return nil
}

func (*noopStreamProvider) Close() error {
	return nil
}
