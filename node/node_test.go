/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewWithConfPathE(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "core.yaml"), []byte("fsc:\n  id: test-node\n"), 0o600)
	require.NoError(t, err)

	n, err := NewWithConfPathE(dir)
	require.NoError(t, err)
	require.NotNil(t, n)
	require.Equal(t, "test-node", n.ID())
}

func TestNewWithConfPathE_InvalidPath(t *testing.T) {
	t.Parallel()

	_, err := NewWithConfPathE("./does-not-exist")
	require.Error(t, err)
}
