/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const subtreeYAML = `
fsc:
  tls:
    enabled: true
    clientAuthRequired: false
  grpc:
    tls:
      clientRootCAs:
        files:
          - ca.crt
`

func TestRawSubtree(t *testing.T) {
	t.Parallel()

	p, err := (&Provider{}).ProvideFromRaw([]byte(subtreeYAML))
	require.NoError(t, err)

	// Callers pass camelCase; the backend stores lowercase.
	sub, ok := p.RawSubtree("fsc.grpc.tls")
	require.True(t, ok)
	require.Contains(t, sub, "clientrootcas")

	// An intermediate path exists because koanf populates key parts.
	sub, ok = p.RawSubtree("fsc.tls")
	require.True(t, ok)
	require.Equal(t, true, sub["enabled"])
	require.Equal(t, false, sub["clientauthrequired"])

	// Absent must be false, NOT an empty map with true — koanf's Cut cannot tell those
	// apart, which is why RawSubtree uses Exists.
	sub, ok = p.RawSubtree("fsc.web.tls")
	require.False(t, ok)
	require.Nil(t, sub)

	// A leaf is not a subtree.
	sub, ok = p.RawSubtree("fsc.tls.enabled")
	require.False(t, ok)
	require.Nil(t, sub)
}

func TestStrictUnmarshalSubtreeRejectsUnknownKey(t *testing.T) {
	t.Parallel()

	type target struct {
		Enabled *bool `yaml:"enabled"`
	}

	var ok target
	require.NoError(t, StrictUnmarshalSubtree(map[string]any{"enabled": true}, &ok))
	require.True(t, *ok.Enabled)

	var bad target
	err := StrictUnmarshalSubtree(map[string]any{"enabldd": true}, &bad)
	require.ErrorContains(t, err, "enabldd")
}

// The file-reading hooks must not be installed: they read a path before TranslatePath has
// run. A {file: ...} map decoded into a *struct* field must stay a path, not become bytes.
func TestStrictUnmarshalSubtreeDoesNotReadFiles(t *testing.T) {
	t.Parallel()

	type inner struct {
		File string `yaml:"file"`
	}
	type target struct {
		Cert *inner `yaml:"cert"`
	}

	var got target
	err := StrictUnmarshalSubtree(map[string]any{
		"cert": map[string]any{"file": "does-not-exist.crt"},
	}, &got)
	require.NoError(t, err, "decoding must not touch the filesystem")
	require.Equal(t, "does-not-exist.crt", got.Cert.File)
}
