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

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
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

// Node keys are absolute, so the prefix narrows the search. They are also leaf values, not
// subtrees: detecting only subtrees silently passed the whole migration table once already.
func TestCheckRemovedNodeKeys(t *testing.T) {
	t.Parallel()
	set := fakeSource{dir: t.TempDir(), subtrees: map[string]map[string]any{
		"fsc.metrics.prometheus": {"tls": true},
	}}

	err := CheckRemovedKeys(set, "fsc")
	require.ErrorContains(t, err, "has been removed")
	require.ErrorContains(t, err, "fsc.metrics.prometheus.tls")
	require.ErrorContains(t, err, "fsc.metrics.clientAuthRequired")

	require.NoError(t, CheckRemovedKeys(set, "fsc.p2p"),
		"a narrower prefix must not reach a key outside it")
	require.NoError(t, CheckRemovedKeys(fakeSource{dir: t.TempDir()}, "fsc"),
		"a configuration without the key must pass")
}

// Network entries are RELATIVE to their network, so the prefix is prepended to reach the
// configured key. An earlier version filtered by it instead — and
// strings.HasPrefix("ordering.tlsenabled", "fabric.mynetwork.") is always false, so every
// network entry was skipped unconditionally and this returned nil for the config below.
//
// The value being false is the point: ordering.tlsEnabled: false used to disable TLS for
// orderer connections alone. Now that the network block covers them, a leftover setting
// silently flips plaintext to TLS unless it is rejected here, so presence has to be detected
// regardless of the value.
func TestCheckRemovedNetworkKeys(t *testing.T) {
	t.Parallel()
	// Both prefix shapes NewService produces: a named network, and the default one.
	for _, prefix := range []string{"fabric.mynetwork.", "fabric."} {
		src := fakeSource{dir: t.TempDir(), subtrees: map[string]map[string]any{
			prefix + "ordering": {"tlsenabled": false},
		}}
		err := CheckRemovedNetworkKeys(src, prefix)
		require.ErrorContains(t, err, "has been removed")
		require.ErrorContains(t, err, prefix+"ordering.tlsenabled",
			"the error must name the key as the operator wrote it")
		require.ErrorContains(t, err, prefix+"tls.enabled",
			"and the replacement, qualified to the same network")
	}

	require.NoError(t, CheckRemovedNetworkKeys(fakeSource{
		dir: t.TempDir(),
		subtrees: map[string]map[string]any{
			"fabric.mynetwork.ordering": {"numretries": 3},
		},
	}, "fabric.mynetwork."), "a network using only supported keys must pass")
}

// The two scopes must not reach into each other: a node key is not a network key with a
// prefix, and unifying the tables would make fsc.tls.clientAuthRequired — a supported key
// that every NWO node sets — look like the removed fabric.<net>.tls.clientAuthRequired.
func TestCheckRemovedKeysScopesAreSeparate(t *testing.T) {
	t.Parallel()
	network := fakeSource{dir: t.TempDir(), subtrees: map[string]map[string]any{
		"fabric.mynetwork.ordering": {"tlsenabled": false},
	}}
	require.NoError(t, CheckRemovedKeys(network, "fsc"),
		"the node check must not see a network key")
	require.NoError(t, CheckRemovedKeys(network, "fabric.mynetwork."),
		"nor find one by being handed a network prefix")

	node := fakeSource{dir: t.TempDir(), subtrees: map[string]map[string]any{
		"fsc.metrics.prometheus": {"tls": true},
		"fsc.tls":                {"clientauthrequired": false},
	}}
	require.NoError(t, CheckRemovedNetworkKeys(node, "fsc."),
		"the network check must not reject a supported fsc.tls field")
}
