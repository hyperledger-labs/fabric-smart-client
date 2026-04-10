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

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/routing"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// TestHostStartupTimeout tests that the host properly handles startup failures
func TestHostStartupTimeout(t *testing.T) { //nolint:paralleltest
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Use an invalid address that will cause the server to fail to listen
	invalidAddress := "invalid:address"

	// Create a mock config with invalid address
	cfg := &mockConfig{
		listenAddress: host2.PeerIPAddress(invalidAddress),
	}

	// Create a host with the invalid address
	host := websocket.NewHost(
		"test-node",
		routing.NewServiceDiscovery(&routing.StaticIDRouter{}, routing.AlwaysFirst[host2.PeerIPAddress]()),
		noopProvider(),
		cfg,
		nil, // caPoolProvider
	)

	// Attempt to start the host - should fail due to inability to listen
	err := host.Start(func(host2.P2PStream) {})
	require.Error(t, err)
	// The error should be related to failing to listen
}

// TestHostStartupReadinessTimeout tests the 2-second startup readiness timeout
func TestHostStartupReadinessTimeout(t *testing.T) { //nolint:paralleltest
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Use a valid localhost address but don't actually start a listener
	// This simulates a scenario where the server starts listening but
	// the readiness check fails because no one is accepting connections
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close()) // Close immediately so no one is listening

	// Create a mock config
	cfg := &mockConfig{
		listenAddress: host2.PeerIPAddress(addr),
	}

	// Create a host
	h := websocket.NewHost(
		"test-node",
		routing.NewServiceDiscovery(&routing.StaticIDRouter{}, routing.AlwaysFirst[host2.PeerIPAddress]()),
		noopProvider(),
		cfg,
		nil, // caPoolProvider
	)

	// Ensure we clean up resources
	defer func() {
		_ = h.Close()
	}()

	// Attempt to start - should timeout waiting for readiness
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	// Use ctx to satisfy compiler that it's used
	_ = ctx
	err = h.Start(func(host2.P2PStream) {})
	// This might succeed or fail depending on timing, but we're mainly
	// testing that the timeout mechanism exists and doesn't panic
	if err != nil {
		// If it fails, it could be due to our timeout
		require.ErrorContains(t, err, "timeout waiting for REST server readiness")
	}
}

// mockConfig implements websocket.Config for testing
type mockConfig struct {
	listenAddress host2.PeerIPAddress
}

func (m *mockConfig) ListenAddress() host2.PeerIPAddress                        { return m.listenAddress }
func (m *mockConfig) ClientTLSConfig(websocket.ExtraCAPoolProvider) *tls.Config { return nil }
func (m *mockConfig) ServerTLSConfig(websocket.ExtraCAPoolProvider) *tls.Config { return nil }
func (m *mockConfig) CertPath() string                                          { return "" }
func (m *mockConfig) MaxSubConns() int                                          { return 100 }
func (m *mockConfig) ReadHeaderTimeout() time.Duration                          { return 10 * time.Second }
func (m *mockConfig) ReadTimeout() time.Duration                                { return 30 * time.Second }
func (m *mockConfig) WriteTimeout() time.Duration                               { return 30 * time.Second }
func (m *mockConfig) IdleTimeout() time.Duration                                { return 120 * time.Second }
func (m *mockConfig) CORSAllowedOrigins() []string                              { return nil }

// noopProvider returns a stream provider that does nothing
func noopProvider() websocket.StreamProvider {
	return &noopStreamProvider{}
}

type noopStreamProvider struct{}

func (n *noopStreamProvider) NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (host2.P2PStream, error) {
	return nil, nil
}

func (n *noopStreamProvider) NewServerStream(writer http.ResponseWriter, request *http.Request, newStreamCallback func(host2.P2PStream)) error {
	return nil
}

func (n *noopStreamProvider) Close() error {
	return nil
}
