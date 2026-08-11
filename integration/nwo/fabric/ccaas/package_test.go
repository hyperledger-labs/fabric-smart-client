/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccaas

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPackage(t *testing.T) { //nolint:paralleltest
	out := filepath.Join(t.TempDir(), "ns.tar.gz")
	err := BuildPackage(out, "mycc", Connection{Address: "127.0.0.1:7052", DialTimeout: "10s"})
	if err != nil {
		t.Fatalf("BuildPackage: %v", err)
	}

	files := readTarGz(t, out)
	// metadata.json
	var md struct{ Path, Type, Label string }
	if err := json.Unmarshal(files["metadata.json"], &md); err != nil {
		t.Fatalf("metadata.json: %v", err)
	}
	if md.Type != "ccaas" || md.Label != "mycc" {
		t.Fatalf("bad metadata: %+v", md)
	}
	// code.tar.gz -> connection.json
	conn := readTarGzBytes(t, files["code.tar.gz"])["connection.json"]
	var c Connection
	if err := json.Unmarshal(conn, &c); err != nil {
		t.Fatalf("connection.json: %v", err)
	}
	if c.Address != "127.0.0.1:7052" || c.TLSRequired {
		t.Fatalf("bad connection: %+v", c)
	}
}

func TestBuildPackageIsDeterministic(t *testing.T) { //nolint:paralleltest
	dir := t.TempDir()
	conn := Connection{Address: "127.0.0.1:9999", DialTimeout: "10s"}

	first := filepath.Join(dir, "a.tar.gz")
	second := filepath.Join(dir, "b.tar.gz")
	require.NoError(t, BuildPackage(first, "events", conn))
	require.NoError(t, BuildPackage(second, "events", conn))

	a, err := os.ReadFile(first)
	require.NoError(t, err)
	b, err := os.ReadFile(second)
	require.NoError(t, err)

	// The package id is the sha256 of this file, and each org's peers must
	// agree on it, so the same connection must produce the same bytes.
	require.Equal(t, sha256.Sum256(a), sha256.Sum256(b))
}

func TestBuildPackageDiffersByAddress(t *testing.T) { //nolint:paralleltest
	dir := t.TempDir()

	org1 := filepath.Join(dir, "org1.tar.gz")
	org2 := filepath.Join(dir, "org2.tar.gz")
	require.NoError(t, BuildPackage(org1, "events",
		Connection{Address: "127.0.0.1:9001", DialTimeout: "10s"}))
	require.NoError(t, BuildPackage(org2, "events",
		Connection{Address: "127.0.0.1:9002", DialTimeout: "10s"}))

	a, err := os.ReadFile(org1)
	require.NoError(t, err)
	b, err := os.ReadFile(org2)
	require.NoError(t, err)

	// Different addresses must yield different package ids; the per-org
	// deployment depends on it.
	require.NotEqual(t, sha256.Sum256(a), sha256.Sum256(b))
}

func readTarGz(t *testing.T, path string) map[string][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	return readTarGzReader(t, f)
}

func readTarGzBytes(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	return readTarGzReader(t, bytesReader(b))
}

func readTarGzReader(t *testing.T, r io.Reader) map[string][]byte {
	t.Helper()
	gr, err := gzip.NewReader(r)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gr)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(tr)
		out[h.Name] = data
	}
	return out
}

func bytesReader(b []byte) io.Reader {
	return bytes.NewReader(b)
}
