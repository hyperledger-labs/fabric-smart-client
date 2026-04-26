/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "core.yaml"), []byte("fsc:\n  id: node1\n"), 0o600)
	require.NoError(t, err)

	usedConfig, err := Validate(dir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "core.yaml"), usedConfig)
}

func TestValidateMissingID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "core.yaml"), []byte("logging:\n  spec: info\n"), 0o600)
	require.NoError(t, err)

	_, err = Validate(dir)
	require.EqualError(t, err, "missing fsc.id in configuration")
}

func TestCmd(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "core.yaml"), []byte("fsc:\n  id: node1\n"), 0o600)
	require.NoError(t, err)

	cmd := Cmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config-path", dir})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, out.String(), filepath.Join(dir, "core.yaml"))
}
