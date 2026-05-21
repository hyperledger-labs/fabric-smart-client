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

func TestValidateConfigCommand(t *testing.T) {
	t.Parallel()

	confPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(confPath, "core.yaml"), []byte(validConfigYAML(t)), 0o600))

	cmd := NewCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"config", "--path", confPath})

	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "configuration is valid")
	require.Contains(t, out.String(), "validated fsc.grpc server configuration")
}
