/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// TestClientStreamBindsServerCertificateToRemotePeerID dials peer V at a server presenting
// peer A's certificate, which the client trusts and which is valid for the dialed host. No
// stream to V may open, neither on a fresh connection nor on a cached connection to A.
func TestClientStreamBindsServerCertificateToRemotePeerID(t *testing.T) {
	t.Parallel()
	testSetup(t)

	serverTLSConfig, clientTLSConfig, clientID := testMutualTLSConfigs(t, false)
	peerA := serverPeerID(t, serverTLSConfig)
	vKey, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	require.NoError(t, err)
	vPub, err := x509.MarshalPKIXPublicKey(&vKey.PublicKey)
	require.NoError(t, err)
	vHash := sha256.Sum256(vPub)
	peerV := base64.StdEncoding.EncodeToString(vHash[:])

	srv := startTestServer(t, NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0), serverTLSConfig, func(s host.P2PStream) { _ = s.Close() })
	t.Cleanup(srv.Close)
	info := func(peer host.PeerID) host.StreamInfo {
		return host.StreamInfo{
			RemotePeerID:      peer,
			RemotePeerAddress: strings.TrimPrefix(srv.URL, "https://"),
			ContextID:         "ctx",
			SessionID:         "session",
		}
	}

	t.Run("multiplexed", func(t *testing.T) {
		t.Parallel()
		p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
		t.Cleanup(func() { _ = p.Close() })

		_, err := p.NewClientStream(info(peerV), t.Context(), clientID, clientTLSConfig)
		require.ErrorContains(t, err, "peer identity binding failed")
		p.mu.RLock()
		require.Empty(t, p.clients, "a connection that failed the binding must not be cached")
		p.mu.RUnlock()

		// A cached connection to A must not carry a stream meant for V.
		s, err := p.NewClientStream(info(peerA), t.Context(), clientID, clientTLSConfig)
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Close() })
		_, err = p.NewClientStream(info(peerV), t.Context(), clientID, clientTLSConfig)
		require.ErrorContains(t, err, "peer identity binding failed")
	})

	t.Run("simple", func(t *testing.T) {
		t.Parallel()
		_, err := NewSimpleProvider().NewClientStream(info(peerV), t.Context(), clientID, clientTLSConfig)
		require.ErrorContains(t, err, "peer identity binding failed")
	})
}
