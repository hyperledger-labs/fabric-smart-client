/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

// tlsCapturingStreamProvider records the TLS configuration of the last outbound stream.
type tlsCapturingStreamProvider struct {
	StreamProvider

	config *tls.Config
}

func (p *tlsCapturingStreamProvider) NewClientStream(_ host2.StreamInfo, _ context.Context, _ host2.PeerID, config *tls.Config) (host2.P2PStream, error) {
	p.config = config
	return nil, nil
}

// Every outbound stream trusts exactly the extra CAs the provider supplies at that moment.
func TestClientRootCAsFollowExtraCAs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	selfCert, selfKey, err := GenerateTestCert("self")
	require.NoError(t, err)
	certPath, keyPath := filepath.Join(dir, "self.crt"), filepath.Join(dir, "self.key")
	require.NoError(t, os.WriteFile(certPath, selfCert, 0o600))
	require.NoError(t, os.WriteFile(keyPath, selfKey, 0o600))
	cfg, err := NewConfigFromProperties("127.0.0.1:0", keyPath, certPath, nil, nil, true, 0, nil)
	require.NoError(t, err)

	peerPEM, _, err := GenerateTestCert("peer")
	require.NoError(t, err)
	block, _ := pem.Decode(peerPEM)
	peerCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	known := extraCAs{peerPEM} // trusted at startup
	sp := &tlsCapturingStreamProvider{}
	h, err := NewHost("self", nil, sp, cfg, &known)
	require.NoError(t, err)
	open := func() *tls.Config {
		_, err := h.client.OpenStream(host2.StreamInfo{RemotePeerAddress: "127.0.0.1:9999"}, t.Context())
		require.NoError(t, err)
		require.Equal(t, "127.0.0.1", sp.config.ServerName)
		return sp.config
	}
	trustsPeer := func(c *tls.Config) bool {
		_, err := peerCert.Verify(x509.VerifyOptions{Roots: c.RootCAs})
		return err == nil
	}

	require.True(t, trustsPeer(open()))

	// A withdrawn anchor is no longer trusted: the startup extra CAs are not baked in.
	known = extraCAs{}
	require.False(t, trustsPeer(open()))

	// A peer discovered after startup is trusted, and an entry that is not a certificate
	// (such as the node's own resolver ID) does not break the dial.
	known = extraCAs{[]byte("not a pem"), peerPEM}
	first := open()
	require.True(t, trustsPeer(first))

	// While the extra CAs are unchanged, streams share one pool and get their own config.
	second := open()
	require.Same(t, first.RootCAs, second.RootCAs)
	require.NotSame(t, first, second)
	require.NotSame(t, h.client.tlsConfig, second)
}
