/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadPemFile_InvalidPEMContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "bad.pem")
	require.NoError(t, os.WriteFile(f, []byte("not pem content"), 0o644))
	_, err := readPemFile(f)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no pem content")
}

func TestReadFile_NonExistent(t *testing.T) {
	t.Parallel()
	_, err := readFile("/nonexistent/file.txt")
	require.Error(t, err)
}

func TestReadPemFile_Success(t *testing.T) {
	t.Parallel()
	raw, err := readPemFile(filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"))
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}
