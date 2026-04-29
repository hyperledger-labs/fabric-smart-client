/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validate

import (
	"bytes"
	"errors"
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

func TestValidateMissingConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_, err := Validate(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "core.yaml")
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

func TestCmdTrailingArgs(t *testing.T) {
	t.Parallel()

	cmd := Cmd()
	cmd.SetArgs([]string{"unexpected"})

	err := cmd.Execute()
	require.EqualError(t, err, "trailing args detected")
	require.False(t, cmd.SilenceUsage)
}

func TestCmdValidateError(t *testing.T) {
	t.Parallel()

	cmd := Cmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config-path", t.TempDir()})

	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "core.yaml")
	require.True(t, cmd.SilenceUsage)
}

func TestCmdWriteError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "core.yaml"), []byte("fsc:\n  id: node1\n"), 0o600)
	require.NoError(t, err)

	cmd := Cmd()
	cmd.SetOut(failingWriter{})
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"--config-path", dir})

	err = cmd.Execute()
	require.EqualError(t, err, "write failed")
}

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}
