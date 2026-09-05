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

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/tlsgen"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

// providerFrom writes a core.yaml plus a real keypair into a temp dir and loads it, so the
// resolution path under test is the real one: lowercased keys, TranslatePath, file reads.
func providerFrom(t *testing.T, body string) *config.Provider {
	t.Helper()
	dir := t.TempDir()

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	kp, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "server.crt"), kp.Cert, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "server.key"), kp.Key, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.crt"), ca.CertBytes(), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core.yaml"), []byte(body), 0o600))

	p, err := config.NewProvider(dir)
	require.NoError(t, err)
	return p
}

// fsc.grpc.tls inherits enabled, cert and key from fsc.tls and overrides only the mTLS
// fields — the shape the NWO template emits.
func TestNewServerConfigInheritsFromFscTLS(t *testing.T) {
	t.Parallel()
	p := providerFrom(t, `
fsc:
  tls:
    enabled: true
    cert:
      file: server.crt
    key:
      file: server.key
    clientAuthRequired: false
  grpc:
    enabled: true
    address: 127.0.0.1:0
    tls:
      clientAuthRequired: true
      clientRootCAs:
        files:
          - ca.crt
`)

	got, err := NewServerConfig(p)
	require.NoError(t, err)
	require.True(t, got.SecOpts.UseTLS, "enabled inherited from fsc.tls")
	require.NotEmpty(t, got.SecOpts.Certificate)
	require.NotEmpty(t, got.SecOpts.Key)
	require.True(t, got.SecOpts.RequireClientCert)
	require.Len(t, got.SecOpts.ClientRootCAs, 1)
}

// The web listener resolves through the same parent, and keeps its three-way client auth.
func TestNewWebServerTLSOptionalClientAuth(t *testing.T) {
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
    tls:
      clientRootCAs:
        files:
          - ca.crt
`)

	got, err := resolveWebTLS(p)
	require.NoError(t, err)
	require.True(t, got.UseTLS)
	require.False(t, got.RequireClientCert)

	cfg, err := got.TLSConfig()
	require.NoError(t, err)
	require.Equal(t, tls.VerifyClientCertIfGiven, cfg.ClientAuth,
		"client root CAs without clientAuthRequired must still verify a cert if offered")
}

// A misspelled key under a tls: block must fail startup, naming the key.
func TestNewServerConfigRejectsMisspelledKey(t *testing.T) {
	t.Parallel()
	p := providerFrom(t, `
fsc:
  grpc:
    enabled: true
    address: 127.0.0.1:0
    tls:
      enabled: true
      clientAuthRequird: true
`)

	_, err := NewServerConfig(p)
	require.ErrorContains(t, err, "clientauthrequird")
}

// A removed key must be named alongside its replacement.
func TestCheckTLSConfigRejectsRemovedKey(t *testing.T) {
	t.Parallel()
	p := providerFrom(t, `
fsc:
  p2p:
    opts:
      websocket:
        tls:
          serverRootCAs:
            files:
              - ca.crt
`)

	err := CheckTLSConfig(p)
	require.ErrorContains(t, err, "has been removed")
	require.ErrorContains(t, err, "rootCAs.files")
}
