/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/tlsgen"
)

// fakeSource is a Source over a literal nested map, so resolution can be tested without a
// config file. Keys are lowercased, matching what the real provider stores.
type fakeSource struct {
	subtrees map[string]map[string]any
	arrays   map[string][]map[string]any
	dir      string
}

func (f fakeSource) RawSubtrees(key string) []map[string]any { return f.arrays[key] }

func (f fakeSource) RawSubtree(key string) (map[string]any, bool) {
	m, ok := f.subtrees[key]
	return m, ok
}

func (f fakeSource) IsSet(key string) bool {
	if _, ok := f.subtrees[key]; ok {
		return true
	}
	// Leaf keys: present when some subtree holds the last segment.
	i := strings.LastIndex(key, ".")
	if i < 0 {
		return false
	}
	if m, ok := f.subtrees[key[:i]]; ok {
		_, ok = m[key[i+1:]]
		return ok
	}
	return false
}

func (f fakeSource) TranslatePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(f.dir, path)
}

// writeKeyPair drops a usable server cert, key and CA into dir and returns their names.
func writeKeyPair(t *testing.T, dir string) (cert, key, ca string) {
	t.Helper()
	authority, err := tlsgen.NewCA()
	require.NoError(t, err)
	kp, err := authority.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "s.crt"), kp.Cert, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "s.key"), kp.Key, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.crt"), authority.CertBytes(), 0o600))
	return "s.crt", "s.key", "ca.crt"
}

func TestResolveServerInheritsFromParent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, ca := writeKeyPair(t, dir)

	src := fakeSource{dir: dir, subtrees: map[string]map[string]any{
		"fsc.tls": {
			"enabled": true,
			"cert":    map[string]any{"file": cert},
			"key":     map[string]any{"file": key},
		},
		"fsc.grpc.tls": {
			"clientauthrequired": true,
			"clientrootcas":      map[string]any{"files": []any{ca}},
		},
	}}

	got, err := ResolveServer(src, "fsc.tls", "fsc.grpc.tls")
	require.NoError(t, err)
	require.True(t, got.UseTLS, "enabled inherited from fsc.tls")
	require.NotEmpty(t, got.Certificate, "cert inherited and loaded")
	require.NotEmpty(t, got.Key)
	require.True(t, got.RequireClientCert)
	require.Len(t, got.ClientRootCAs, 1)
}

func TestResolveServerAbsentChildIsFine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, _ := writeKeyPair(t, dir)

	src := fakeSource{dir: dir, subtrees: map[string]map[string]any{
		"fsc.tls": {
			"enabled": true,
			"cert":    map[string]any{"file": cert},
			"key":     map[string]any{"file": key},
		},
	}}

	got, err := ResolveServer(src, "fsc.tls", "fsc.web.tls")
	require.NoError(t, err)
	require.True(t, got.UseTLS, "a surface with no block of its own is the parent's")
}

// Per-surface defaults: websocket's is true, and only websocket's.
func TestResolveServerClientAuthDefaults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, ca := writeKeyPair(t, dir)
	base := map[string]any{
		"enabled":       true,
		"cert":          map[string]any{"file": cert},
		"key":           map[string]any{"file": key},
		"clientrootcas": map[string]any{"files": []any{ca}},
	}
	src := fakeSource{dir: dir, subtrees: map[string]map[string]any{"fsc.tls": base}}

	// Websocket resolves through its own entry point, which makes mutual TLS mandatory.
	ws, _, err := ResolveWebsocketP2P(src, "fsc.p2p.opts.websocket.tls", nil, nil)
	require.NoError(t, err)
	require.True(t, ws.RequireClientCert, "websocket requires client certificates")

	rpc, err := ResolveServer(src, "fsc.tls", "fsc.grpc.tls")
	require.NoError(t, err)
	require.False(t, rpc.RequireClientCert, "every other surface defaults to false")
}

// An explicit false suppresses inherited client credentials.
func TestResolveClientExplicitFalseSuppressesInheritedCreds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, ca := writeKeyPair(t, dir)

	src := fakeSource{dir: dir, subtrees: map[string]map[string]any{
		"fabric.mynet.tls": {
			"enabled":    true,
			"rootcas":    map[string]any{"files": []any{ca}},
			"clientcert": map[string]any{"file": cert},
			"clientkey":  map[string]any{"file": key},
		},
	}}

	inherited, err := ResolveClient(src, "fabric.mynet.tls")
	require.NoError(t, err)
	require.True(t, inherited.RequireClientCert,
		"clientAuthEnabled defaults true when both clientCert and clientKey resolve")
	require.NotEmpty(t, inherited.Certificate)

	suppressed, err := ResolveEndpointClient(src, "fabric.mynet.tls",
		map[string]any{"clientauthenabled": false})
	require.NoError(t, err)
	require.False(t, suppressed.RequireClientCert)
	require.Empty(t, suppressed.Certificate, "suppressed means the cert is not presented")
}

// clientCert/clientKey fall back to cert/key, which is what makes one composite block
// usable for websocket P2P without restating the keypair.
func TestResolveBothFallsBackToServerKeypair(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, ca := writeKeyPair(t, dir)

	src := fakeSource{dir: dir, subtrees: map[string]map[string]any{
		"fsc.p2p.opts.websocket.tls": {
			"enabled":       true,
			"cert":          map[string]any{"file": cert},
			"key":           map[string]any{"file": key},
			"rootcas":       map[string]any{"files": []any{ca}},
			"clientrootcas": map[string]any{"files": []any{ca}},
		},
	}}

	server, client, err := ResolveWebsocketP2P(src, "fsc.p2p.opts.websocket.tls", nil, nil)
	require.NoError(t, err)
	require.Equal(t, server.Certificate, client.Certificate, "clientCert falls back to cert")
	require.True(t, client.RequireClientCert, "and so clientAuthEnabled defaults true")
	require.True(t, server.RequireClientCert)
	require.Len(t, server.ClientRootCAs, 1, "server verifies inbound peers")
	require.Len(t, client.ServerRootCAs, 1, "client verifies outbound servers")
}

// A misspelling under a tls: subtree is an error, not a silent discard. This is the whole
// point of the refactor.
func TestResolveRejectsUnknownKey(t *testing.T) {
	t.Parallel()
	src := fakeSource{dir: t.TempDir(), subtrees: map[string]map[string]any{
		"fsc.grpc.tls": {"clientauthrequird": true},
	}}

	_, err := ResolveServer(src, "fsc.tls", "fsc.grpc.tls")
	require.ErrorContains(t, err, "clientauthrequird")
	require.ErrorContains(t, err, "fsc.grpc.tls")
}

// A client-shaped key under a server-shaped block is caught the same way.
func TestResolveServerRejectsClientOnlyKey(t *testing.T) {
	t.Parallel()
	src := fakeSource{dir: t.TempDir(), subtrees: map[string]map[string]any{
		"fsc.grpc.tls": {"rootcas": map[string]any{"files": []any{"ca.crt"}}},
	}}

	_, err := ResolveServer(src, "fsc.tls", "fsc.grpc.tls")
	require.ErrorContains(t, err, "rootcas")
}

func TestResolveServerNameOverrideInherits(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, _, ca := writeKeyPair(t, dir)

	src := fakeSource{dir: dir, subtrees: map[string]map[string]any{
		"fabric.mynet.tls": {
			"enabled":            true,
			"rootcas":            map[string]any{"files": []any{ca}},
			"servernameoverride": "orderer.example.com",
		},
	}}

	inherited, err := ResolveEndpointClient(src, "fabric.mynet.tls", nil)
	require.NoError(t, err)
	require.Equal(t, "orderer.example.com", inherited.ServerNameOverride,
		"an endpoint with no block of its own is the network block's")

	overridden, err := ResolveEndpointClient(src, "fabric.mynet.tls",
		map[string]any{"servernameoverride": "orderer0.example.com"})
	require.NoError(t, err)
	require.Equal(t, "orderer0.example.com", overridden.ServerNameOverride)
}
