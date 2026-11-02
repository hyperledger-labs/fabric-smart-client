/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	commongrpc "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	web "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
)

// servedLeaf returns the leaf certificate a fresh TLS handshake with addr gets. No SNI is sent
// (addr is an IP), which is the case where crypto/tls would prefer Certificates[0] over
// GetCertificate.
func servedLeaf(t *testing.T, addr string) []byte {
	t.Helper()
	c, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // only the served certificate matters
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	return c.ConnectionState().PeerCertificates[0].Raw
}

// Both default listeners serve a certificate renewed on disk from the next handshake on,
// once their reloader has re-read the files, and report its expiry.
func TestListenersServeReloadedCertificate(t *testing.T) {
	t.Parallel()
	p, ca := providerWithCA(t, `
fsc:
  tls:
    enabled: true
    cert:
      file: server.crt
    key:
      file: server.key
    reloadInterval: 1h
  grpc:
    enabled: true
    address: 127.0.0.1:0
  web:
    enabled: true
    address: 127.0.0.1:0
`)
	ws, err := NewWebServer(p)
	require.NoError(t, err)
	require.NoError(t, ws.Start())
	t.Cleanup(func() { _ = ws.Stop() })
	gs, err := NewGRPCServer(p)
	require.NoError(t, err)
	go func() { _ = gs.Start() }()
	t.Cleanup(gs.Stop)

	addrs := map[string]string{
		"web":  ws.(*web.Server).Addr(),
		"grpc": gs.(*commongrpc.GRPCServer).Address(),
	}
	before := servedLeaf(t, addrs["web"])

	kp, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	dir := filepath.Dir(p.TranslatePath("server.crt"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "server.crt"), kp.Cert, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "server.key"), kp.Key, 0o600))

	for name, addr := range addrs {
		require.Equal(t, before, servedLeaf(t, addr), "%s: not re-read before the interval", name)
	}
	require.NoError(t, ws.(certReloaderOwner).CertReloader().Reload())
	require.NoError(t, gs.(certReloaderOwner).CertReloader().Reload())
	for name, addr := range addrs {
		require.NotEqual(t, before, servedLeaf(t, addr), "%s: renewed certificate on the next handshake", name)
	}

	gauges := map[string]*mock.Gauge{}
	provider := &mock.Provider{}
	provider.NewGaugeCalls(func(o metrics.GaugeOpts) metrics.Gauge {
		require.Equal(t, "fsc", o.Namespace)
		require.Equal(t, "tls", o.Subsystem)
		require.Equal(t, "cert_not_after", o.Name)
		parent := &mock.Gauge{}
		parent.WithCalls(func(lv ...string) metrics.Gauge {
			g := &mock.Gauge{}
			gauges[lv[1]] = g
			return g
		})
		return parent
	})
	reportCertExpiry(provider, gs, ws, nil)
	require.Len(t, gauges, 2, "no metrics label without an own operations listener")
	for name, owner := range map[string]any{"grpc": gs, "web": ws} {
		want := owner.(certReloaderOwner).CertReloader().NotAfter().Unix()
		require.NotZero(t, want)
		require.Equal(t, want, int64(gauges[name].SetArgsForCall(0)), name)
	}
}
