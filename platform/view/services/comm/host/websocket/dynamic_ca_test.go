/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gorilla_websocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/ws"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type mockEndpointService struct {
	mu        sync.RWMutex
	resolvers []endpoint.ResolverInfo
}

func (*mockEndpointService) ExtractPKI(id []byte) []byte {
	return id
}

func (m *mockEndpointService) Resolvers() []endpoint.ResolverInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.resolvers
}

func (m *mockEndpointService) AddResolver(id []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolvers = append(m.resolvers, endpoint.ResolverInfo{ID: id})
}

func (m *mockEndpointService) UpdateResolver(name, domain string, addresses map[string]string, _ []string, id []byte) (view.Identity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	for i, r := range m.resolvers {
		if r.Name == name {
			m.resolvers[i].Addresses = convert(addresses)
			found = true
			break
		}
	}
	if !found {
		m.resolvers = append(m.resolvers, endpoint.ResolverInfo{
			Name:      name,
			Domain:    domain,
			Addresses: convert(addresses),
			ID:        id,
		})
	}
	return id, nil
}

func convert(o map[string]string) map[endpoint.PortName]string {
	r := map[endpoint.PortName]string{}
	for k, v := range o {
		r[endpoint.PortName(k)] = v
	}
	return r
}

func (*mockEndpointService) GetIdentity(_ string, _ []byte) (view.Identity, error) {
	return nil, nil
}

func (m *mockEndpointService) GetResolver(_ context.Context, id view.Identity) (*endpoint.Resolver, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.resolvers {
		if string(r.ID) == string(id) || r.Name == string(id) {
			return &endpoint.Resolver{ResolverInfo: r}, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func TestDynamicCA(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	// Generate Server Cert
	serverKeyFile := filepath.Join(tempDir, "server.key")
	serverCertFile := filepath.Join(tempDir, "server.crt")
	serverCertPEM, serverKeyPEM, err := websocket.GenerateTestCert("server")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(serverKeyFile, serverKeyPEM, 0o600))
	require.NoError(t, os.WriteFile(serverCertFile, serverCertPEM, 0o600))

	// Generate Client Cert (NOT in initial root CAs)
	clientKeyFile := filepath.Join(tempDir, "client.key")
	clientCertFile := filepath.Join(tempDir, "client.crt")
	clientCertPEM, clientKeyPEM, err := websocket.GenerateTestCert("client")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(clientKeyFile, clientKeyPEM, 0o600))
	require.NoError(t, os.WriteFile(clientCertFile, clientCertPEM, 0o600))

	config, err := websocket.NewConfigFromProperties(
		"127.0.0.1:0",
		serverKeyFile,
		serverCertFile,
		[]string{serverCertFile}, // Server trusts itself
		[]string{},               // Server initially trusts NO clients
		true,                     // Require mTLS
		100, nil,
	)
	require.NoError(t, err)

	epService := &mockEndpointService{}
	r := routing.NewEndpointServiceIDRouter(epService)
	discovery := routing.NewServiceDiscovery(r, routing.Random[host.PeerIPAddress]())

	streamProvider := ws.NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	provider := websocket.NewEndpointBasedProvider(config, epService, discovery, streamProvider)

	h, err := provider.GetNewHost()
	require.NoError(t, err)

	err = h.Start(func(stream host.P2PStream) {
		_ = stream.Close()
	})
	require.NoError(t, err)
	defer func() { _ = h.Close() }()

	// Verify that the endpoint service was updated with the actual address
	actualAddr := h.(interface{ Addr() string }).Addr()
	res, err := epService.GetResolver(t.Context(), []byte(h.(interface{ ID() string }).ID()))
	require.NoError(t, err)
	require.Equal(t, actualAddr, res.Addresses[endpoint.P2PPort])

	// 1. Try to connect with client cert - should fail because it's not in server's root CAs
	clientTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{mustLoadKeyPair(clientCertFile, clientKeyFile)},
		RootCAs:      x509.NewCertPool(),
	}
	clientTLSConfig.RootCAs.AppendCertsFromPEM(serverCertPEM)
	clientTLSConfig.InsecureSkipVerify = false

	// Small wait for server to be ready
	time.Sleep(200 * time.Millisecond)

	serverAddr := h.(interface{ Addr() string }).Addr()
	url := fmt.Sprintf("wss://%s/p2p", serverAddr)

	dialer := &gorilla_websocket.Dialer{TLSClientConfig: clientTLSConfig}
	_, resp, err := dialer.DialContext(t.Context(), url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err, "should fail as client cert is not trusted")

	// 2. Add client cert to EndpointService (runtime change)
	epService.AddResolver(clientCertPEM)

	// 3. Try to connect again - should succeed now!
	conn, resp, err := dialer.DialContext(t.Context(), url, nil)
	if err == nil {
		_ = conn.Close()
		_ = resp.Body.Close()
	}
	require.NoError(t, err, "should succeed as client cert is now trusted via EndpointService")
}

// GetNewHost's own TLS-config check must surface the error, not just NewHost's.
func TestGetNewHostPropagatesTLSConfigErrors(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	certPEM, keyPEM, err := websocket.GenerateTestCert("node")
	require.NoError(t, err)
	certFile, keyFile := filepath.Join(tempDir, "n.crt"), filepath.Join(tempDir, "n.key")
	require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))
	badCAFile := filepath.Join(tempDir, "bad-ca.crt")
	require.NoError(t, os.WriteFile(badCAFile, []byte("not a pem"), 0o600))

	config, err := websocket.NewConfigFromProperties("127.0.0.1:0", keyFile, certFile,
		[]string{badCAFile}, []string{certFile}, true, 100, nil)
	require.NoError(t, err)

	epService := &mockEndpointService{}
	r := routing.NewEndpointServiceIDRouter(epService)
	discovery := routing.NewServiceDiscovery(r, routing.Random[host.PeerIPAddress]())
	streamProvider := ws.NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	provider := websocket.NewEndpointBasedProvider(config, epService, discovery, streamProvider)

	_, err = provider.GetNewHost()
	require.ErrorContains(t, err, "failed to build client TLS config")
}

func mustLoadKeyPair(certFile, keyFile string) tls.Certificate {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		panic(err)
	}
	return cert
}

// syncExtraCAs is an ExtraCAPoolProvider whose anchors can change while hosts use it.
type syncExtraCAs struct {
	mu  sync.Mutex
	cas [][]byte
}

func (s *syncExtraCAs) ExtraCAs() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cas
}

func (s *syncExtraCAs) set(cas ...[]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cas = cas
}

// newTestNode returns the configuration of a mutual-TLS host named name, and its certificate.
func newTestNode(t *testing.T, name string) (websocket.Config, []byte) {
	t.Helper()
	dir := t.TempDir()
	certPEM, keyPEM, err := websocket.GenerateTestCert(name)
	require.NoError(t, err)
	certFile, keyFile := filepath.Join(dir, name+".crt"), filepath.Join(dir, name+".key")
	require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))
	cfg, err := websocket.NewConfigFromProperties("127.0.0.1:0", keyFile, certFile, nil, nil, true, 100, nil)
	require.NoError(t, err)
	return cfg, certPEM
}

// startTestHost starts a host that closes every inbound stream.
func startTestHost(t *testing.T, name string, cfg websocket.Config, cas websocket.ExtraCAPoolProvider) interface{ Addr() string } {
	t.Helper()
	h, err := websocket.NewHost(name, nil, newTestStreamProvider(), cfg, cas)
	require.NoError(t, err)
	require.NoError(t, h.Start(func(stream host.P2PStream) { _ = stream.Close() }))
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func newTestStreamProvider() websocket.StreamProvider {
	return ws.NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
}

// dialTestHost opens and closes one stream from a fresh host named name to the host at addr,
// so no connection is reused between calls.
func dialTestHost(t *testing.T, name string, cfg websocket.Config, cas websocket.ExtraCAPoolProvider, addr string) error {
	t.Helper()
	router := routing.StaticIDRouter{"remote": {addr}}
	h, err := websocket.NewHost(name,
		routing.NewServiceDiscovery(router, routing.AlwaysFirst[host.PeerIPAddress]()),
		newTestStreamProvider(), cfg, cas)
	require.NoError(t, err)
	defer func() { _ = h.Close() }()
	s, err := h.NewStream(t.Context(), host.StreamInfo{RemotePeerID: "remote", ContextID: "ctx", SessionID: "s"})
	if err == nil {
		_ = s.Close()
	}
	return err
}

// An outbound dial trusts a peer whose anchor the provider learns after the host started,
// even when the provider also holds entries that are not certificates.
func TestDynamicCAOutbound(t *testing.T) {
	t.Parallel()
	aliceCfg, aliceCert := newTestNode(t, "alice")
	bobCfg, bobCert := newTestNode(t, "bob")

	// Bob knows Alice from the start, so only Alice's outbound trust is under test.
	bob := startTestHost(t, "bob", bobCfg, &syncExtraCAs{cas: [][]byte{aliceCert}})

	aliceCAs := &syncExtraCAs{}
	require.ErrorContains(t, dialTestHost(t, "alice", aliceCfg, aliceCAs, bob.Addr()), "certificate signed by unknown authority")

	aliceCAs.set([]byte("alice"), bobCert) // Alice's own non-PEM resolver ID, then Bob
	require.NoError(t, dialTestHost(t, "alice", aliceCfg, aliceCAs, bob.Addr()))
}

// A server stops accepting a peer once the provider withdraws its anchor, including an
// anchor that was already known when the server started.
func TestDynamicCAInboundWithdrawal(t *testing.T) {
	t.Parallel()
	aliceCfg, aliceCert := newTestNode(t, "alice")
	bobCfg, bobCert := newTestNode(t, "bob")

	bobCAs := &syncExtraCAs{cas: [][]byte{aliceCert}}
	bob := startTestHost(t, "bob", bobCfg, bobCAs)
	aliceCAs := &syncExtraCAs{cas: [][]byte{bobCert}}
	require.NoError(t, dialTestHost(t, "alice", aliceCfg, aliceCAs, bob.Addr()))

	bobCAs.set()
	require.ErrorContains(t, dialTestHost(t, "alice", aliceCfg, aliceCAs, bob.Addr()), "unknown certificate authority")
}
