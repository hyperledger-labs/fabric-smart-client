/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/tlsgen"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

// writeNodeDir lays out a node directory with an application identity certificate and a
// SEPARATE transport keypair, so a test can tell the two apart.
func writeNodeDir(t *testing.T, body string) (dir string, identityCert, transportCert []byte) {
	t.Helper()
	dir = t.TempDir()

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	identity, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)
	transport, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	require.NotEqual(t, identity.Cert, transport.Cert)

	for name, content := range map[string][]byte{
		"identity.crt": identity.Cert,
		"identity.key": identity.Key,
		"p2p.crt":      transport.Cert,
		"p2p.key":      transport.Key,
		"ca.crt":       ca.CertBytes(),
		"core.yaml":    []byte(body),
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), content, 0o600))
	}
	return dir, identity.Cert, transport.Cert
}

const p2pCore = `
fsc:
  identity:
    cert:
      file: identity.crt
    key:
      file: identity.key
  p2p:
    listenAddress: /ip4/127.0.0.1/tcp/9000
    opts:
      websocket:
        tls:
          enabled: true
          rootCAs:
            files:
              - ca.crt
          clientRootCAs:
            files:
              - ca.crt
`

// The transport keypair IS the identity keypair, and must stay that way: the peer ID a host
// announces is derived from the public key of its verified TLS certificate
// (ws.expectedPeerIDFromRequest), and the receiver rejects a mismatch. Giving P2P a separate
// certificate silently breaks every session with "peer identity binding failed".
func TestTransportKeypairIsTheIdentityKeypair(t *testing.T) {
	t.Parallel()
	dir, identityCert, transportCert := writeNodeDir(t, p2pCore)

	p, err := config2.NewProvider(dir)
	require.NoError(t, err)
	cfg, err := NewConfig(p)
	require.NoError(t, err)

	require.Equal(t, filepath.Join(dir, "identity.crt"), cfg.CertPath(),
		"CertPath feeds nodeID via ExtractPKI and must stay on fsc.identity")
	require.Equal(t, identityCert, cfg.serverTLS.Certificate,
		"the listener must present the identity certificate, or the peer ID binding fails")
	require.Equal(t, identityCert, cfg.clientTLS.Certificate)
	require.NotEqual(t, transportCert, cfg.serverTLS.Certificate)
}

// fsc.tls holds the LISTENER certificate, which is a different credential. The P2P block
// must not inherit it, or the binding above breaks.
func TestP2PDoesNotInheritTheListenerKeypair(t *testing.T) {
	t.Parallel()
	body := `
fsc:
  identity:
    cert:
      file: identity.crt
    key:
      file: identity.key
  tls:
    enabled: true
    cert:
      file: p2p.crt
    key:
      file: p2p.key
  p2p:
    listenAddress: /ip4/127.0.0.1/tcp/9000
    opts:
      websocket:
        tls:
          enabled: true
`
	dir, identityCert, listenerCert := writeNodeDir(t, body)
	p, err := config2.NewProvider(dir)
	require.NoError(t, err)

	cfg, err := NewConfig(p)
	require.NoError(t, err)
	require.Equal(t, identityCert, cfg.serverTLS.Certificate)
	require.NotEqual(t, listenerCert, cfg.serverTLS.Certificate,
		"fsc.tls must not supply the P2P transport keypair")
}

// Websocket P2P has always required mutual TLS.
func TestNewConfigDefaultsToMutualTLS(t *testing.T) {
	t.Parallel()
	dir, _, _ := writeNodeDir(t, p2pCore)

	p, err := config2.NewProvider(dir)
	require.NoError(t, err)
	cfg, err := NewConfig(p)
	require.NoError(t, err)
	require.True(t, cfg.serverTLS.RequireClientCert)
}

// A key that no longer exists, and a key that never did, are both rejected rather than
// silently discarded. The two cases differ only in the tls: block.
func TestNewConfigRejectsBadP2PTLSKeys(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, tlsBlock, wantErr string }{{
		// serverRootCAs was renamed to rootCAs.
		name: "a removed key",
		tlsBlock: "          enabled: true\n" +
			"          serverRootCAs:\n            files:\n              - ca.crt\n",
		wantErr: "serverrootcas",
	}, {
		name:     "a misspelled key",
		tlsBlock: "          clientAuthRequird: true\n",
		wantErr:  "clientauthrequird",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, _, _ := writeNodeDir(t, `
fsc:
  identity:
    cert:
      file: identity.crt
    key:
      file: identity.key
  p2p:
    listenAddress: /ip4/127.0.0.1/tcp/9000
    opts:
      websocket:
        tls:
`+tc.tlsBlock)
			p, err := config2.NewProvider(dir)
			require.NoError(t, err)

			_, err = NewConfig(p)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
