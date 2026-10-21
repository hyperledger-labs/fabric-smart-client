/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package configtest checks the reference configuration in docs/configuration.md against the
// code that reads it. It is imported only by tests.
//
// UnmarshalKey ignores keys that no struct field matches, so a misspelled, misplaced or no
// longer read key in a node's configuration falls back to its default without an error.
// Each platform keeps the reference honest with a test that claims its sections of the
// document: [Reference.Decode], [Reference.DecodeEach] and [Reference.DecodeEndpoints]
// strictly decode the sections the code decodes into a struct, [Reference.Read] lists the keys the code reads one at a time,
// [Reference.Delegate] hands a section to the test of the component that reads it, and
// [Reference.RequireComplete] fails on any documented key under the claimed roots that none
// of these covered.
package configtest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	koanfmaps "github.com/knadh/koanf/maps"
	koanfyaml "github.com/knadh/koanf/parsers/yaml"
	koanfbytes "github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/tlsconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

// Network is the name of the Fabric network documented in the reference.
const Network = "fabric.mynetwork"

// Reference is the first yaml block of docs/configuration.md together with the keys a test
// has covered so far. A Reference is not safe for concurrent use.
type Reference struct {
	*config.Provider

	t         *testing.T
	covered   map[string]bool // lowercase leaf keys
	delegated []string        // lowercase keys whose subtree another test checks
}

// Load returns the reference configuration. It is loaded without config.NewProvider, which
// merges CORE_* environment variables on top of the file and would make the result depend on
// the environment the test runs in.
func Load(t *testing.T) *Reference {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	doc := filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "configuration.md")
	raw, err := os.ReadFile(doc)
	require.NoError(t, err)
	m := regexp.MustCompile("(?s)```yaml\n(.*?)\n```").FindSubmatch(raw)
	require.NotNil(t, m, "%s has no yaml block", doc)

	k := koanf.New(".")
	require.NoError(t, k.Load(koanfbytes.Provider(m[1]), config.LowercaseParser{Parser: koanfyaml.Parser()}))
	return &Reference{Provider: &config.Provider{Backend: k}, t: t, covered: map[string]bool{}}
}

// Check strictly decodes raw into out, failing the test with key in the message if raw holds
// a key that out has no field for. It records no coverage; use it for maps nested in a
// section that is covered otherwise, such as the free-form options of an array element.
func (r *Reference) Check(key string, raw map[string]any, out any) {
	r.t.Helper()
	require.NoError(r.t, config.StrictUnmarshalSubtree(raw, out), "%s does not match %T", key, out)
}

// Decode strictly decodes the subtree at key into out and covers every leaf it decoded. The
// keys in except, relative to key and possibly dotted, are left out of the decode and stay
// uncovered: the test covers them separately, typically because the code resolves them on
// their own, as it does tls: blocks.
func (r *Reference) Decode(key string, out any, except ...string) {
	r.t.Helper()
	raw, ok := r.RawSubtree(key) // a copy: the decode may modify it
	require.True(r.t, ok, "%s is not documented", key)
	for _, e := range except {
		deletePath(raw, strings.ToLower(e))
	}
	r.Check(key, raw, out)
	flat, _ := koanfmaps.Flatten(raw, nil, ".")
	for leaf := range flat {
		r.covered[strings.ToLower(key)+"."+leaf] = true
	}
}

// DecodeEach strictly decodes each element of the array at key into a value from newOut and
// covers the array. An array is a single key in the configuration, so its elements need no
// further coverage.
func (r *Reference) DecodeEach(key string, newOut func() any) {
	r.t.Helper()
	r.decodeElems(key, r.RawSubtrees(key), newOut)
}

// DecodeEndpoints is [Reference.DecodeEach] for an array of grpc.ConnectionConfig. An
// element's tls block is decoded as client TLS, the way tlsconfig.ResolveEndpointClients
// resolves it, and is left out of the element's decode.
func (r *Reference) DecodeEndpoints(key string) {
	r.t.Helper()
	elems := r.RawSubtrees(key) // copies: deleting from them leaves the provider unchanged
	for i, raw := range elems {
		if tls, ok := raw["tls"].(map[string]any); ok {
			r.Check(fmt.Sprintf("%s[%d].tls", key, i), tls, &tlsconfig.ClientTLS{})
			delete(raw, "tls")
		}
	}
	r.decodeElems(key, elems, func() any { return &grpc.ConnectionConfig{} })
}

func (r *Reference) decodeElems(key string, elems []map[string]any, newOut func() any) {
	r.t.Helper()
	require.NotEmpty(r.t, elems, "%s is not documented", key)
	for i, raw := range elems {
		r.Check(fmt.Sprintf("%s[%d]", key, i), raw, newOut())
	}
	r.covered[strings.ToLower(key)] = true
}

// Read covers keys that the code reads one at a time (GetString, GetInt, IsSet, ...), so no
// strict decode can check them. Each key must be a documented leaf.
func (r *Reference) Read(keys ...string) {
	r.t.Helper()
	for _, key := range keys {
		require.True(r.t, r.IsSet(key), "%s is read but not documented", key)
		_, isSubtree := r.RawSubtree(key)
		require.False(r.t, isSubtree, "%s is a section; list its keys", key)
		require.Empty(r.t, r.RawSubtrees(key), "%s is an array; decode its elements", key)
		r.covered[strings.ToLower(key)] = true
	}
}

// Delegate covers the subtrees at keys, which the reference test of the component that reads
// them checks, calling RequireComplete on them.
func (r *Reference) Delegate(keys ...string) {
	r.t.Helper()
	for _, key := range keys {
		require.True(r.t, r.IsSet(key), "%s is delegated but not documented", key)
		r.delegated = append(r.delegated, strings.ToLower(key))
	}
}

// RequireComplete fails the test for every documented key under roots that is neither
// decoded, read, nor delegated.
func (r *Reference) RequireComplete(roots ...string) {
	r.t.Helper()
	for _, key := range r.Backend.Keys() {
		if !slices.ContainsFunc(roots, func(root string) bool { return under(key, strings.ToLower(root)) }) {
			continue
		}
		if !r.covered[key] && !slices.ContainsFunc(r.delegated, func(d string) bool { return under(key, d) }) {
			r.t.Errorf("%s is documented but not checked: decode, read or delegate it", key)
		}
	}
}

// under reports whether key is root or lies below it.
func under(key, root string) bool {
	return key == root || strings.HasPrefix(key, root+".")
}

// deletePath removes the dotted path from m, if present.
func deletePath(m map[string]any, path string) {
	head, rest, nested := strings.Cut(path, ".")
	if !nested {
		delete(m, head)
		return
	}
	if child, ok := m[head].(map[string]any); ok {
		deletePath(child, rest)
	}
}
