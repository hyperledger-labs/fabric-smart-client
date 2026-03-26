/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest_test

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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
)

type mockEndpointService struct {
	mu        sync.RWMutex
	resolvers []endpoint.ResolverInfo
}

func (m *mockEndpointService) ExtractPKI(id []byte) []byte {
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

func (m *mockEndpointService) UpdateResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error) {
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

func (m *mockEndpointService) GetIdentity(label string, pkID []byte) (view.Identity, error) {
	return nil, nil
}

func (m *mockEndpointService) GetResolver(ctx context.Context, id view.Identity) (*endpoint.Resolver, error) {
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
	tempDir := t.TempDir()

	// Generate Server Cert
	serverKeyFile := filepath.Join(tempDir, "server.key")
	serverCertFile := filepath.Join(tempDir, "server.crt")
	serverCertPEM, serverKeyPEM, err := rest.GenerateTestCert("server")
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(serverKeyFile, serverKeyPEM, 0600))
	assert.NoError(t, os.WriteFile(serverCertFile, serverCertPEM, 0600))

	// Generate Client Cert (NOT in initial root CAs)
	clientKeyFile := filepath.Join(tempDir, "client.key")
	clientCertFile := filepath.Join(tempDir, "client.crt")
	clientCertPEM, clientKeyPEM, err := rest.GenerateTestCert("client")
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(clientKeyFile, clientKeyPEM, 0600))
	assert.NoError(t, os.WriteFile(clientCertFile, clientCertPEM, 0600))

	config := rest.NewConfigFromProperties(
		"127.0.0.1:0",
		serverKeyFile,
		serverCertFile,
		[]string{serverCertFile}, // Server trusts itself
		[]string{},               // Server initially trusts NO clients
		true,                     // Require mTLS
		100, nil,
	)

	epService := &mockEndpointService{}
	r := routing.NewEndpointServiceIDRouter(epService)
	discovery := routing.NewServiceDiscovery(r, routing.Random[host.PeerIPAddress]())

	streamProvider := websocket.NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
	provider := rest.NewEndpointBasedProvider(config, epService, discovery, streamProvider)

	h, err := provider.GetNewHost()
	if !assert.NoError(t, err) {
		return
	}

	err = h.Start(func(stream host.P2PStream) {
		_ = stream.Close()
	})
	assert.NoError(t, err)
	defer func() { _ = h.Close() }()

	// Verify that the endpoint service was updated with the actual address
	actualAddr := h.(interface{ Addr() string }).Addr()
	res, err := epService.GetResolver(t.Context(), []byte(h.(interface{ ID() string }).ID()))
	assert.NoError(t, err)
	assert.Equal(t, actualAddr, res.Addresses[endpoint.P2PPort])

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
	_, _, err = dialer.DialContext(t.Context(), url, nil)
	assert.Error(t, err, "should fail as client cert is not trusted")

	// 2. Add client cert to EndpointService (runtime change)
	epService.AddResolver(clientCertPEM)

	// 3. Try to connect again - should succeed now!
	conn, resp, err := dialer.DialContext(t.Context(), url, nil)
	if err == nil {
		_ = conn.Close()
		_ = resp.Body.Close()
	}
	assert.NoError(t, err, "should succeed as client cert is now trusted via EndpointService")
}

func mustLoadKeyPair(certFile, keyFile string) tls.Certificate {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		panic(err)
	}
	return cert
}
