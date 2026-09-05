/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsconfig

import (
	"cmp"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// Source is the configuration this package reads. Satisfied by *config.Provider and by the
// Fabric platform's prefixed Configuration wrapper.
type Source interface {
	// RawSubtree returns the raw map at key, and whether key names a subtree.
	// Implementations lowercase the key themselves; callers pass camelCase.
	RawSubtree(key string) (map[string]any, bool)
	// TranslatePath resolves a configured path relative to the configuration file.
	TranslatePath(path string) string
	// IsSet reports whether the key is present, whether it names a leaf or a subtree.
	// [CheckRemovedKeys] needs this: most removed keys are leaves, which RawSubtree cannot
	// see.
	IsSet(key string) bool
}

// ResolveServer resolves the server half of key, inheriting per field from parentKey. This is
// the only surface with a parent block: every listener may inherit from fsc.tls.
func ResolveServer(src Source, parentKey, key string) (grpc.SecureOptions, error) {
	parentRaw, _ := src.RawSubtree(parentKey)
	parent, err := decode[ServerTLS](parentKey, parentRaw)
	if err != nil {
		return grpc.SecureOptions{}, err
	}
	childRaw, _ := src.RawSubtree(key)
	child, err := decode[ServerTLS](key, childRaw)
	if err != nil {
		return grpc.SecureOptions{}, err
	}
	return buildServer(src, key, mergeServer(parent, child), false, false)
}

// ResolveClient resolves the client half of key.
//
// There is no parent key: a client block is either a surface's own (fsc.tracing.otlp.tls) or
// the network block that per-endpoint blocks inherit from, and neither inherits from anything
// itself. Endpoints do the merging, through [ResolveEndpointClient].
func ResolveClient(src Source, key string) (grpc.SecureOptions, error) {
	raw, _ := src.RawSubtree(key)
	c, err := decode[ClientTLS](key, raw)
	if err != nil {
		return grpc.SecureOptions{}, err
	}
	return buildClient(src, key, c, nil, nil)
}

// ResolveWebsocketP2P resolves the websocket P2P block, returning the SERVER direction first
// and the CLIENT direction second.
//
// It is its own entry point rather than a set of options, because every rule that surface
// follows differs from the templates: mutual TLS is mandatory rather than defaulted, and its
// trust anchors arrive at handshake time rather than from configuration. The one block is read
// once and viewed through both roles, and it has no parent to inherit from.
//
// identityCert and identityKey are the fsc.identity keypair the block falls back to: the peer
// ID a host announces is derived from the public key of the certificate it presents, so the
// transport keypair has to be the node's identity.
func ResolveWebsocketP2P(src Source, key string, identityCert, identityKey *File) (grpc.SecureOptions, grpc.SecureOptions, error) {
	var zero grpc.SecureOptions
	raw, _ := src.RawSubtree(key)
	t, err := decode[TLS](key, raw)
	if err != nil {
		return zero, zero, err
	}
	cert, keyFile := cmp.Or(t.Cert, identityCert), cmp.Or(t.Key, identityKey)

	server, err := buildServer(src, key, ServerTLS{
		Enabled:            t.Enabled,
		Cert:               cert,
		Key:                keyFile,
		ClientAuthRequired: t.ClientAuthRequired,
		ClientRootCAs:      t.ClientRootCAs,
	}, true, true)
	if err != nil {
		return zero, zero, err
	}
	// The client half falls back to the server half's keypair, identity default included.
	client, err := buildClient(src, key, ClientTLS{
		Enabled:            t.Enabled,
		RootCAs:            t.RootCAs,
		ClientAuthEnabled:  t.ClientAuthEnabled,
		ClientCert:         t.ClientCert,
		ClientKey:          t.ClientKey,
		ServerNameOverride: t.ServerNameOverride,
	}, cert, keyFile)
	if err != nil {
		return zero, zero, err
	}
	return server, client, nil
}

// decode strictly decodes one subtree. An unknown key is an error naming the key and the
// subtree — the whole point of scoping ErrorUnused to tls: blocks.
func decode[T any](key string, raw map[string]any) (T, error) {
	var out T
	if len(raw) == 0 {
		return out, nil
	}
	if err := config.StrictUnmarshalSubtree(raw, &out); err != nil {
		return out, errors.Wrapf(err, "invalid TLS configuration under [%s]", key)
	}
	return out, nil
}

// buildServer loads and validates the server direction. defaultClientAuth is used when neither
// the block nor its parent set clientAuthRequired. dynamicClientRootCAs records that trust
// anchors arrive at handshake time rather than from configuration, which makes the
// "clientAuthRequired with an empty pool" check unsound — it holds only for a static pool.
func buildServer(src Source, key string, s ServerTLS, defaultClientAuth, dynamicClientRootCAs bool) (grpc.SecureOptions, error) {
	out := grpc.SecureOptions{
		UseTLS:            deref(s.Enabled, false),
		RequireClientCert: deref(s.ClientAuthRequired, defaultClientAuth),
	}
	var err error
	if out.Certificate, err = readFile(src, key, "cert", s.Cert); err != nil {
		return out, err
	}
	if out.Key, err = readFile(src, key, "key", s.Key); err != nil {
		return out, err
	}
	if out.ClientRootCAs, err = readFiles(src, key, "clientRootCAs", s.ClientRootCAs); err != nil {
		return out, err
	}
	return out, validateServer(key, out, dynamicClientRootCAs)
}

func buildClient(src Source, key string, c ClientTLS, fallbackCert, fallbackKey *File) (grpc.SecureOptions, error) {
	cert, keyFile := cmp.Or(c.ClientCert, fallbackCert), cmp.Or(c.ClientKey, fallbackKey)

	out := grpc.SecureOptions{
		UseTLS:             deref(c.Enabled, false),
		ServerNameOverride: deref(c.ServerNameOverride, ""),
	}
	var err error
	if out.Certificate, err = readFile(src, key, "clientCert", cert); err != nil {
		return out, err
	}
	if out.Key, err = readFile(src, key, "clientKey", keyFile); err != nil {
		return out, err
	}
	if out.ServerRootCAs, err = readFiles(src, key, "rootCAs", c.RootCAs); err != nil {
		return out, err
	}

	// Default true iff both halves of the keypair resolved; an explicit false suppresses
	// inherited credentials, which is the only way an endpoint can say "do not present a
	// certificate to this one".
	haveKeypair := len(out.Certificate) > 0 && len(out.Key) > 0
	out.RequireClientCert = deref(c.ClientAuthEnabled, haveKeypair)
	if !out.RequireClientCert {
		out.Certificate, out.Key = nil, nil
	}
	return out, validateClient(key, out, cert, keyFile)
}

func deref[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

func readFile(src Source, key, field string, f *File) ([]byte, error) {
	if f == nil || f.File == "" {
		return nil, nil
	}
	path := src.TranslatePath(f.File)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "%s.%s: cannot read [%s]", key, field, path)
	}
	return b, nil
}

func readFiles(src Source, key, field string, f *Files) ([][]byte, error) {
	if f == nil {
		return nil, nil
	}
	out := make([][]byte, 0, len(f.Files))
	for _, p := range f.Files {
		path := src.TranslatePath(p)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.Wrapf(err, "%s.%s: cannot read [%s]", key, field, path)
		}
		out = append(out, b)
	}
	return out, nil
}

// The real provider must satisfy Source, or every consumer needs an adapter.
var _ Source = (*config.Provider)(nil)
