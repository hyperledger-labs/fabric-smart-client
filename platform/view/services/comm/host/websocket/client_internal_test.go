/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"crypto/tls"
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

// Outbound connections trust extra CAs the provider discovers after the host is built.
func TestClientTrustsExtraCAsDiscoveredAfterStartup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	selfCert, selfKey, err := GenerateTestCert("self")
	require.NoError(t, err)
	certPath, keyPath := filepath.Join(dir, "self.crt"), filepath.Join(dir, "self.key")
	require.NoError(t, os.WriteFile(certPath, selfCert, 0o600))
	require.NoError(t, os.WriteFile(keyPath, selfKey, 0o600))
	cfg, err := NewConfigFromProperties("127.0.0.1:0", keyPath, certPath, nil, nil, true, 0, nil)
	require.NoError(t, err)

	peerCert, peerKey, err := GenerateTestCert("peer")
	require.NoError(t, err)
	peerKeyPair, err := tls.X509KeyPair(peerCert, peerKey)
	require.NoError(t, err)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{peerKeyPair},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.(*tls.Conn).Handshake()
			_ = conn.Close()
		}
	}()

	known := extraCAs{}
	sp := &tlsCapturingStreamProvider{}
	h, err := NewHost("self", nil, sp, cfg, &known)
	require.NoError(t, err)
	dial := func() error {
		_, err := h.client.OpenStream(host2.StreamInfo{RemotePeerAddress: ln.Addr().String()}, t.Context())
		require.NoError(t, err)
		conn, err := tls.Dial("tcp", ln.Addr().String(), sp.config)
		if err == nil {
			_ = conn.Close()
		}
		return err
	}

	require.ErrorContains(t, dial(), "certificate signed by unknown authority")

	known = extraCAs{peerCert} // the peer is discovered after startup
	require.NoError(t, dial())
}
