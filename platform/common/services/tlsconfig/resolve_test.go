/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsconfig

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestResolveServerVersionRange is the ServerTLS half of
// TestResolveEndpointClientVersionRange: the version range flows through mergeServer and
// buildServer, which are separate functions from the client-side merge/build with their own
// field lists, so the client-side test does not exercise this path.
func TestResolveServerVersionRange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, _ := writeKeyPair(t, dir)

	src := fakeSource{dir: dir, subtrees: map[string]map[string]any{
		"fsc.tls": {
			"enabled":    true,
			"cert":       map[string]any{"file": cert},
			"key":        map[string]any{"file": key},
			"minversion": 772,
		},
	}}

	t.Run("inherited from the parent block", func(t *testing.T) {
		t.Parallel()
		got, err := ResolveServer(src, "fsc.tls", "fsc.grpc.tls")
		require.NoError(t, err)
		require.Equal(t, uint16(tls.VersionTLS13), got.MinVersion)
		require.Zero(t, got.MaxVersion, "an unset ceiling stays unset")
	})

	t.Run("overridden by the child block", func(t *testing.T) {
		t.Parallel()
		override := fakeSource{dir: dir, subtrees: map[string]map[string]any{
			"fsc.tls":      src.subtrees["fsc.tls"],
			"fsc.grpc.tls": {"minversion": 771},
		}}
		got, err := ResolveServer(override, "fsc.tls", "fsc.grpc.tls")
		require.NoError(t, err)
		require.Equal(t, uint16(tls.VersionTLS12), got.MinVersion, "child's minVersion wins over the parent's")
	})

	t.Run("maxVersion resolves too, not just minVersion", func(t *testing.T) {
		t.Parallel()
		ceiling := fakeSource{dir: dir, subtrees: map[string]map[string]any{
			"fsc.tls": {
				"enabled":    true,
				"cert":       map[string]any{"file": cert},
				"key":        map[string]any{"file": key},
				"minversion": 772,
				"maxversion": 772,
			},
		}}
		got, err := ResolveServer(ceiling, "fsc.tls", "fsc.grpc.tls")
		require.NoError(t, err)
		require.Equal(t, uint16(tls.VersionTLS13), got.MaxVersion, "an explicit ceiling is not dropped")
	})
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
	ws, _, err := ResolveWebsocketP2P(src, "fsc.p2p.opts.websocket.tls",
		&File{File: cert}, &File{File: key})
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

// The websocket P2P block uses the fsc.identity keypair in both directions and runs mutual
// TLS without enabled being set. Naming the identity files again changes nothing.
func TestResolveWebsocketP2PUsesTheIdentityKeypair(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, ca := writeKeyPair(t, dir)
	cas := map[string]any{"files": []any{ca}}
	pemCert, err := os.ReadFile(filepath.Join(dir, cert))
	require.NoError(t, err)
	pemCA, err := os.ReadFile(filepath.Join(dir, ca))
	require.NoError(t, err)
	// Copies of the identity certificate that differ in bytes but not in their first
	// certificate, which is the one the peer ID is derived from.
	for name, content := range map[string][]byte{
		"copy.crt":  pemCert,
		"chain.crt": append(append([]byte{}, pemCert...), pemCA...),
		"crlf.crt":  []byte(strings.ReplaceAll(string(pemCert), "\n", "\r\n") + "\r\n"),
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), content, 0o600))
	}
	copyOf := func(file string) map[string]any {
		return map[string]any{
			"cert":       map[string]any{"file": file},
			"clientcert": map[string]any{"file": file},
		}
	}

	for name, block := range map[string]map[string]any{
		"identity by default":  {"rootcas": cas, "clientrootcas": cas},
		"copy at another path": copyOf("copy.crt"),
		"copy with its chain":  copyOf("chain.crt"),
		"copy with CRLF":       copyOf("crlf.crt"),
		"identity restated": {
			"enabled":            true,
			"clientauthrequired": true,
			"clientauthenabled":  true,
			"cert":               map[string]any{"file": cert},
			"key":                map[string]any{"file": key},
			"clientcert":         map[string]any{"file": cert},
			"clientkey":          map[string]any{"file": key},
			"rootcas":            cas,
			"clientrootcas":      cas,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := fakeSource{dir: dir, subtrees: map[string]map[string]any{
				"fsc.p2p.opts.websocket.tls": block,
			}}
			server, client, err := ResolveWebsocketP2P(src, "fsc.p2p.opts.websocket.tls",
				&File{File: cert}, &File{File: key})
			require.NoError(t, err)
			require.True(t, server.UseTLS)
			require.True(t, client.UseTLS)
			require.True(t, server.RequireClientCert, "server verifies inbound peers")
			require.True(t, client.RequireClientCert, "client presents its certificate")
			require.NotEmpty(t, server.Certificate)
			require.Equal(t, server.Certificate, client.Certificate)
			if block["rootcas"] != nil {
				require.Len(t, server.ClientRootCAs, 1, "server verifies inbound peers")
				require.Len(t, client.ServerRootCAs, 1, "client verifies outbound servers")
			}
		})
	}
}

// A websocket P2P key that cannot take effect is an error naming the key: mutual TLS is
// mandatory, and the transport certificate has to be the fsc.identity one, because the peer
// ID a node announces is derived from it.
func TestResolveWebsocketP2PRejectsKeysThatCannotTakeEffect(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, _ := writeKeyPair(t, dir)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "other"), 0o700))
	otherCert, otherKey, _ := writeKeyPair(t, filepath.Join(dir, "other"))
	otherCert, otherKey = filepath.Join("other", otherCert), filepath.Join("other", otherKey)

	for _, tc := range []struct {
		name    string
		block   map[string]any
		wantErr string
	}{
		{"enabled false", map[string]any{"enabled": false}, "enabled"},
		{"clientAuthEnabled false", map[string]any{"clientauthenabled": false}, "clientAuthEnabled"},
		{"clientAuthRequired false", map[string]any{"clientauthrequired": false}, "clientAuthRequired"},
		{"another cert", map[string]any{
			"cert": map[string]any{"file": otherCert},
			"key":  map[string]any{"file": otherKey},
		}, "cert"},
		{"another clientCert", map[string]any{
			"clientcert": map[string]any{"file": otherCert},
			"clientkey":  map[string]any{"file": otherKey},
		}, "clientCert"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := fakeSource{dir: dir, subtrees: map[string]map[string]any{
				"fsc.p2p.opts.websocket.tls": tc.block,
			}}
			_, _, err := ResolveWebsocketP2P(src, "fsc.p2p.opts.websocket.tls",
				&File{File: cert}, &File{File: key})
			require.ErrorContains(t, err, "[fsc.p2p.opts.websocket.tls] "+tc.wantErr+" ")
		})
	}
}

// Websocket P2P presents the node identity, so a missing or unreadable one is reported under
// fsc.identity rather than under a websocket key the operator never set.
func TestResolveWebsocketP2PNamesAMissingIdentity(t *testing.T) {
	t.Parallel()
	src := fakeSource{dir: t.TempDir()}

	_, _, err := ResolveWebsocketP2P(src, "fsc.p2p.opts.websocket.tls", nil, nil)
	require.ErrorContains(t, err, "set fsc.identity.cert.file and fsc.identity.key.file")

	_, _, err = ResolveWebsocketP2P(src, "fsc.p2p.opts.websocket.tls",
		&File{File: "missing.crt"}, &File{File: "missing.key"})
	require.ErrorContains(t, err, "fsc.identity.cert.file: cannot read")
}

// A misspelling under a tls: subtree is an error naming the key, not a silent discard that
// leaves the surface running on a default nobody chose.
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

// reloadInterval inherits from fsc.tls like every other field, a listener can override it,
// and the resolved options carry the file paths the listener re-reads.
func TestResolveServerReloadInterval(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, _ := writeKeyPair(t, dir)
	src := func(child map[string]any) fakeSource {
		return fakeSource{dir: dir, subtrees: map[string]map[string]any{
			"fsc.tls": {
				"enabled":        true,
				"cert":           map[string]any{"file": cert},
				"key":            map[string]any{"file": key},
				"reloadinterval": "30s",
			},
			"fsc.grpc.tls": child,
		}}
	}

	got, err := ResolveServer(src(nil), "fsc.tls", "fsc.grpc.tls")
	require.NoError(t, err)
	require.Equal(t, 30*time.Second, got.ReloadInterval, "inherited from fsc.tls")
	require.Equal(t, filepath.Join(dir, cert), got.CertFile)
	require.Equal(t, filepath.Join(dir, key), got.KeyFile)

	got, err = ResolveServer(src(map[string]any{"reloadinterval": "0s"}), "fsc.tls", "fsc.grpc.tls")
	require.NoError(t, err)
	require.Zero(t, got.ReloadInterval, "an explicit 0 turns an inherited interval off")

	for _, bad := range []string{"-1s", "30"} {
		_, err = ResolveServer(src(map[string]any{"reloadinterval": bad}), "fsc.tls", "fsc.grpc.tls")
		require.ErrorContains(t, err, "reloadInterval", bad)
		require.ErrorContains(t, err, "fsc.grpc.tls", bad)
	}
}
