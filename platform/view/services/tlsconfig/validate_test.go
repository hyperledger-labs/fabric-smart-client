/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type resolveFn func(Source) (grpc.SecureOptions, error)

func server(key string) resolveFn {
	return func(s Source) (grpc.SecureOptions, error) { return ResolveServer(s, "fsc.tls", key) }
}

func client(key string) resolveFn {
	return func(s Source) (grpc.SecureOptions, error) { return ResolveClient(s, key) }
}

func TestValidate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, ca := writeKeyPair(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "junk.crt"), []byte("nope"), 0o600))
	file := func(p string) map[string]any { return map[string]any{"file": p} }
	files := func(p string) map[string]any { return map[string]any{"files": []any{p}} }

	for _, tc := range []struct {
		name    string
		subtree map[string]any
		at      string
		resolve func(string) resolveFn
		wantErr []string
		check   func(*testing.T, grpc.SecureOptions)
	}{{
		name:    "enabled without a keypair is an error naming the block",
		at:      "fsc.grpc.tls",
		resolve: server,
		subtree: map[string]any{"enabled": true},
		wantErr: []string{"cert.file or key.file is missing", "fsc.grpc.tls"},
	}, {
		// The #1111 regression, named so it cannot be deleted by accident.
		name:    "clientAuthRequired with an empty pool is an error (regression #1111)",
		at:      "fsc.grpc.tls",
		resolve: server,
		subtree: map[string]any{
			"enabled": true, "cert": file(cert), "key": file(key), "clientauthrequired": true,
		},
		wantErr: []string{"no client certificate could ever verify"},
	}, {
		name:    "exactly one half of the client keypair is an error",
		at:      "fabric.mynet.tls",
		resolve: client,
		subtree: map[string]any{"enabled": true, "rootcas": files(ca), "clientcert": file(cert)},
		wantErr: []string{"set both or neither"},
	}, {
		name:    "clientAuthEnabled without a keypair is an error",
		at:      "fabric.mynet.tls",
		resolve: client,
		subtree: map[string]any{"enabled": true, "rootcas": files(ca), "clientauthenabled": true},
		wantErr: []string{"clientAuthEnabled is true but"},
	}, {
		name:    "a missing file fails at startup, not at first connection",
		at:      "fsc.grpc.tls",
		resolve: server,
		subtree: map[string]any{"enabled": true, "cert": file("nope.crt"), "key": file("nope.key")},
		wantErr: []string{"cannot read", "nope.crt"},
	}, {
		name:    "a non-PEM CA is rejected before any listener binds",
		at:      "fsc.grpc.tls",
		resolve: server,
		subtree: map[string]any{
			"enabled": true, "cert": file(cert), "key": file(key),
			"clientauthrequired": true, "clientrootcas": files("junk.crt"),
		},
		// Resolution loads the bytes; TLSConfig rejects the non-PEM CA. Both run before a
		// listener binds, which is the guarantee that matters.
		check: func(t *testing.T, so grpc.SecureOptions) {
			t.Helper()
			_, err := so.TLSConfig()
			require.ErrorContains(t, err, "not a valid PEM block")
		},
	}, {
		// enabled:false with a keypair present warns rather than failing — a mistake worth
		// naming, not a reason to refuse to start.
		name:    "disabled with material present still resolves",
		at:      "fsc.grpc.tls",
		resolve: server,
		subtree: map[string]any{"enabled": false, "cert": file(cert), "key": file(key)},
		check: func(t *testing.T, so grpc.SecureOptions) {
			t.Helper()
			require.False(t, so.UseTLS)
		},
	}, {
		// The web listener's supported "verify if offered" state is not an error.
		name:    "client root CAs without clientAuthRequired is fine",
		at:      "fsc.web.tls",
		resolve: server,
		subtree: map[string]any{
			"enabled": true, "cert": file(cert), "key": file(key), "clientrootcas": files(ca),
		},
		check: func(t *testing.T, so grpc.SecureOptions) {
			t.Helper()
			require.False(t, so.RequireClientCert)
			require.Len(t, so.ClientRootCAs, 1)
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := fakeSource{dir: dir, subtrees: map[string]map[string]any{tc.at: tc.subtree}}
			got, err := tc.resolve(tc.at)(src)
			if len(tc.wantErr) > 0 {
				for _, want := range tc.wantErr {
					require.ErrorContains(t, err, want)
				}
				return
			}
			require.NoError(t, err)
			tc.check(t, got)
		})
	}
}

func TestCheckRemovedKeysNamesReplacement(t *testing.T) {
	t.Parallel()
	src := fakeSource{dir: t.TempDir(), subtrees: map[string]map[string]any{
		"fsc.p2p.opts.websocket.tls.serverrootcas": {"files": []any{"ca.crt"}},
	}}
	err := CheckRemovedKeys(src, "fsc")
	require.ErrorContains(t, err, "has been removed")
	require.ErrorContains(t, err, "fsc.p2p.opts.websocket.tls.rootCAs.files")
}

func TestCheckRemovedKeysCleanConfigPasses(t *testing.T) {
	t.Parallel()
	src := fakeSource{dir: t.TempDir(), subtrees: map[string]map[string]any{
		"fsc.p2p.opts.websocket.tls": {"rootcas": map[string]any{"files": []any{"ca.crt"}}},
	}}
	require.NoError(t, CheckRemovedKeys(src, "fsc"))
}

// A phase must not reject keys belonging to a phase that has not landed.
func TestCheckRemovedKeysRespectsPrefix(t *testing.T) {
	t.Parallel()
	src := fakeSource{dir: t.TempDir(), subtrees: map[string]map[string]any{
		"fsc.p2p.opts.websocket.tls.serverrootcas": {"files": []any{"ca.crt"}},
	}}
	require.NoError(t, CheckRemovedKeys(src, "fabric"),
		"a fabric-scoped check must ignore fsc keys")
}

// Most removed keys are leaf values, not subtrees. Detecting only subtrees silently passed
// almost the entire migration table, so this pins the leaf case — using a key the phase files
// really register, rather than mutating the table at runtime.
func TestCheckRemovedKeysFindsLeafKeys(t *testing.T) {
	t.Parallel()
	src := fakeSource{dir: t.TempDir(), subtrees: map[string]map[string]any{
		"fsc.metrics.prometheus": {"tls": true},
	}}
	err := CheckRemovedKeys(src, "fsc")
	require.ErrorContains(t, err, "fsc.metrics.prometheus.tls")
	require.ErrorContains(t, err, "fsc.metrics.clientAuthRequired")

	require.NoError(t, CheckRemovedKeys(fakeSource{dir: t.TempDir()}, "fsc"),
		"a configuration without the key must pass")
}
