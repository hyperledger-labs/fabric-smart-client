/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsconfig

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveEndpointClientInheritsFromNetwork(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, ca := writeKeyPair(t, dir)

	network := fakeSource{dir: dir, subtrees: map[string]map[string]any{
		"fabric.mynet.tls": {
			"enabled":            true,
			"rootcas":            map[string]any{"files": []any{ca}},
			"clientcert":         map[string]any{"file": cert},
			"clientkey":          map[string]any{"file": key},
			"servernameoverride": "default-host",
		},
	}}

	// No block of its own: the endpoint is the network's.
	got, err := ResolveEndpointClient(network, "fabric.mynet.tls", nil)
	require.NoError(t, err)
	require.True(t, got.UseTLS)
	require.Equal(t, "default-host", got.ServerNameOverride)
	require.NotEmpty(t, got.Certificate)

	// A block of its own overrides only what it sets.
	got, err = ResolveEndpointClient(network, "fabric.mynet.tls",
		map[string]any{"servernameoverride": "orderer0"})
	require.NoError(t, err)
	require.Equal(t, "orderer0", got.ServerNameOverride)
	require.True(t, got.UseTLS, "everything else still inherited")
	require.Len(t, got.ServerRootCAs, 1)

	// An explicit false suppresses inherited client credentials for this endpoint alone.
	got, err = ResolveEndpointClient(network, "fabric.mynet.tls",
		map[string]any{"clientauthenabled": false})
	require.NoError(t, err)
	require.False(t, got.RequireClientCert)
	require.Empty(t, got.Certificate)
}

func TestResolveEndpointClientRejectsUnknownKey(t *testing.T) {
	t.Parallel()
	network := fakeSource{dir: t.TempDir(), subtrees: map[string]map[string]any{}}

	_, err := ResolveEndpointClient(network, "fabric.mynet.tls",
		map[string]any{"servernameoveride": "typo"})
	require.ErrorContains(t, err, "servernameoveride")
}

// A raw array shorter than the decoded endpoints means the two reads of the same array
// disagree; resolving anyway would hand the trailing endpoints the network block in place of
// their own, and weaken the connection silently.
func TestResolveEndpointClientsRejectsShortRawArray(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, _, ca := writeKeyPair(t, dir)
	src := fakeSource{
		dir: dir,
		subtrees: map[string]map[string]any{
			"fabric.mynet.tls": {"enabled": true, "rootcas": map[string]any{"files": []any{ca}}},
		},
		arrays: map[string][]map[string]any{
			"fabric.mynet.orderers": {{"address": "o0:7050"}},
		},
	}

	_, err := ResolveEndpointClients(src, "fabric.mynet.tls", "fabric.mynet.orderers", 3)
	require.ErrorContains(t, err, "disagree")

	// An array the source does not expose at all is not a disagreement: every endpoint
	// inherits the network block, which is a valid configuration.
	got, err := ResolveEndpointClients(src, "fabric.mynet.tls", "fabric.mynet.peers", 2)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.True(t, got[0].UseTLS, "inherited from the network block")
	require.True(t, got[1].UseTLS)
}

// TestResolveEndpointClientVersionRange is the configuration half of the TLS 1.3 requirement
// that Fabric-x's committer clients used to express by forking their own credentials builder.
func TestResolveEndpointClientVersionRange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, _, ca := writeKeyPair(t, dir)

	network := fakeSource{dir: dir, subtrees: map[string]map[string]any{
		"fabric.mynet.tls": {
			"enabled":    true,
			"rootcas":    map[string]any{"files": []any{ca}},
			"minversion": 772,
		},
	}}

	t.Run("inherited from the network block", func(t *testing.T) {
		t.Parallel()
		got, err := ResolveEndpointClient(network, "fabric.mynet.tls", nil)
		require.NoError(t, err)
		require.Equal(t, uint16(tls.VersionTLS13), got.MinVersion)
		require.Zero(t, got.MaxVersion, "an unset ceiling stays unset")
	})

	t.Run("overridden per endpoint", func(t *testing.T) {
		t.Parallel()
		got, err := ResolveEndpointClient(network, "fabric.mynet.tls",
			map[string]any{"minversion": 771})
		require.NoError(t, err)
		require.Equal(t, uint16(tls.VersionTLS12), got.MinVersion)
		require.True(t, got.UseTLS, "everything else still inherited")
	})

	t.Run("absent everywhere leaves the default", func(t *testing.T) {
		t.Parallel()
		plain := fakeSource{dir: dir, subtrees: map[string]map[string]any{
			"fabric.mynet.tls": {"enabled": true, "rootcas": map[string]any{"files": []any{ca}}},
		}}
		got, err := ResolveEndpointClient(plain, "fabric.mynet.tls", nil)
		require.NoError(t, err)
		require.Zero(t, got.MinVersion)
		require.Zero(t, got.MaxVersion)

		// Zero is what SecureOptions.TLSConfig turns into the 1.2-1.3 default.
		cfg, err := got.TLSConfig()
		require.NoError(t, err)
		require.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
		require.Equal(t, uint16(tls.VersionTLS13), cfg.MaxVersion)
	})
}
