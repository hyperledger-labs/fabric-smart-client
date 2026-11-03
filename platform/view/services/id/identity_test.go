/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id

import (
	"bytes"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadFile(t *testing.T) {
	t.Parallel()

	t.Run("missing file returns error", func(t *testing.T) {
		t.Parallel()
		_, err := readFile(filepath.Join(t.TempDir(), "missing.pem"))
		require.ErrorContains(t, err, "could not read file")
	})
}

func TestReadPemFile(t *testing.T) {
	t.Parallel()

	t.Run("invalid content returns error", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "notpem.txt")
		err := os.WriteFile(path, []byte("not pem"), 0o600)
		require.NoError(t, err)

		_, readErr := readPemFile(path)
		require.ErrorContains(t, readErr, "no pem content")
	})
}

func TestGetCertFromPem(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns error", func(t *testing.T) {
		t.Parallel()
		_, err := getCertFromPem(nil)
		require.ErrorContains(t, err, "nil idBytes")
	})

	t.Run("invalid pem bytes returns error", func(t *testing.T) {
		t.Parallel()
		_, err := getCertFromPem([]byte("bad pem bytes"))
		require.ErrorContains(t, err, "could not decode pem bytes")
	})

	t.Run("non certificate pem returns parse error", func(t *testing.T) {
		t.Parallel()
		raw, err := os.ReadFile(filepath.Join("testdata", "default", "keystore", "priv_sk"))
		require.NoError(t, err)

		_, certErr := getCertFromPem(raw)
		require.ErrorContains(t, certErr, "failed to parse x509 cert")
	})
}

func TestLoadIdentityFailurePaths(t *testing.T) {
	t.Parallel()

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		_, err := LoadIdentity(filepath.Join(t.TempDir(), "missing.pem"))
		require.Error(t, err)
	})

	t.Run("invalid pem", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "bad.pem")
		err := os.WriteFile(path, []byte("bad"), 0o600)
		require.NoError(t, err)

		_, loadErr := LoadIdentity(path)
		require.ErrorContains(t, loadErr, "no pem content")
	})
}

func TestCanonicalIdentity(t *testing.T) {
	t.Parallel()

	canonical, err := LoadIdentity("./testdata/default/signcerts/default.pem")
	require.NoError(t, err)
	block, _ := pem.Decode(canonical)
	require.NotNil(t, block)

	withHeaders := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Headers: map[string]string{"Comment": "x"}, Bytes: block.Bytes})
	for name, raw := range map[string][]byte{
		"canonical":    canonical,
		"crlf":         bytes.ReplaceAll(canonical, []byte("\n"), []byte("\r\n")),
		"leading text": append([]byte("subject=CN=default\n"), canonical...),
		"headers":      withHeaders,
		"chain":        append(append([]byte{}, canonical...), canonical...),
	} {
		require.Equal(t, canonical, CanonicalIdentity(raw), name)
	}

	for name, raw := range map[string][]byte{
		"nil":          nil,
		"not pem":      []byte("node-id"),
		"pem not cert": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("junk")}),
	} {
		require.Equal(t, raw, CanonicalIdentity(raw), name)
	}
}
