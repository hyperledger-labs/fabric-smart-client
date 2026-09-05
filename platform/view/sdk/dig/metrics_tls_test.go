/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tlsconfig"
)

// fsc.metrics.tls is independent of fsc.web.tls. Before phase 2 the operations endpoints rode
// the web listener and keyed off its enabled flag, so a plaintext metrics endpoint behind a
// TLS web listener could not be expressed.
func TestMetricsTLSIsIndependentOfWeb(t *testing.T) {
	t.Parallel()
	p := providerFrom(t, `
fsc:
  tls:
    enabled: true
    cert:
      file: server.crt
    key:
      file: server.key
  web:
    enabled: true
    address: 127.0.0.1:0
  metrics:
    address: 127.0.0.1:0
    provider: prometheus
    tls:
      enabled: false
`)

	ops, err := NewOperationsServer(p, nil)
	require.NoError(t, err)
	require.NotNil(t, ops.Own, "an address of its own means a listener of its own")

	metricsTLS, err := tlsconfig.ResolveServer(p, "fsc.tls", "fsc.metrics.tls")
	require.NoError(t, err)
	require.False(t, metricsTLS.UseTLS, "explicit false must beat the inherited true")

	web, err := resolveWebTLS(p)
	require.NoError(t, err)
	require.True(t, web.UseTLS, "and must not disturb the sibling listener")
}

// Without an address the operations endpoints stay on the web listener, as today. Silently
// dropping /logspec and /metrics would be a regression, so this stays supported.
func TestOperationsSharesWebListenerWhenNoAddress(t *testing.T) {
	t.Parallel()
	p := providerFrom(t, `
fsc:
  web:
    enabled: true
    address: 127.0.0.1:0
  metrics:
    provider: prometheus
`)

	ops, err := NewOperationsServer(p, nil)
	require.NoError(t, err)
	require.Nil(t, ops.Own, "no address means no listener of its own")
}

// A metrics TLS block that cannot be honoured is an error, not a warning: the configuration
// would otherwise claim transport security the shared listener does not provide.
func TestMetricsTLSWithoutAddressIsAnError(t *testing.T) {
	t.Parallel()
	p := providerFrom(t, `
fsc:
  web:
    enabled: true
    address: 127.0.0.1:0
  metrics:
    provider: prometheus
    tls:
      enabled: true
`)

	_, err := NewOperationsServer(p, nil)
	require.ErrorContains(t, err, "fsc.metrics.address")
}

// fsc.metrics.prometheus.tls is removed; it never meant transport TLS, only "require a client
// certificate to scrape". The error must name its replacement, fsc.metrics.clientAuthRequired.
func TestRemovedPrometheusTLSKeyIsRejected(t *testing.T) {
	t.Parallel()
	p := providerFrom(t, `
fsc:
  metrics:
    provider: prometheus
    prometheus:
      tls: true
`)

	err := CheckTLSConfig(p)
	require.ErrorContains(t, err, "has been removed")
	require.ErrorContains(t, err, "fsc.metrics.clientAuthRequired")
}
