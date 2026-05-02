/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

type recordingServer struct {
	paths []string
}

func (r *recordingServer) RegisterHandler(path string, _ http.Handler, _ bool) {
	r.paths = append(r.paths, path)
}

func (r *recordingServer) Start() error { return nil }

func (r *recordingServer) Stop() error { return nil }

func TestNewOperationsOptionsStructuredMetricsTLS(t *testing.T) {
	t.Parallel()

	provider := newTestConfigProvider(t, `fsc:
  web:
    tls:
      enabled: true
  metrics:
    provider: prometheus
    prometheus:
      address: 127.0.0.1:3000
      tls:
        enabled: true
        clientAuthRequired: true
`)

	opts, err := NewOperationsOptions(provider)
	require.NoError(t, err)
	require.True(t, opts.TLS.Enabled)
	require.Equal(t, "prometheus", opts.Metrics.Provider)
	require.True(t, opts.Metrics.TLS)
}

func TestNewOperationsOptionsLegacyMetricsTLS(t *testing.T) {
	t.Parallel()

	provider := newTestConfigProvider(t, `fsc:
  web:
    tls:
      enabled: false
  metrics:
    provider: prometheus
    prometheus:
      tls: true
`)

	opts, err := NewOperationsOptions(provider)
	require.NoError(t, err)
	require.False(t, opts.TLS.Enabled)
	require.True(t, opts.Metrics.TLS)
}

func TestOperationsServerRoutesMetricsToDedicatedServer(t *testing.T) {
	t.Parallel()

	webServer := &recordingServer{}
	metricsServer := &MetricsServer{
		Server:  &recordingServer{},
		enabled: true,
	}

	router := NewOperationsServer(webServer, metricsServer)
	router.RegisterHandler("/metrics", http.NotFoundHandler(), true)
	router.RegisterHandler("/logspec", http.NotFoundHandler(), true)

	require.Equal(t, []string{"/logspec"}, webServer.paths)
	require.Equal(t, []string{"/metrics"}, metricsServer.Server.(*recordingServer).paths)
}

func newTestConfigProvider(t *testing.T, raw string) *config.Provider {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "core.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	provider, err := config.NewProvider(dir)
	require.NoError(t, err)
	return provider
}
