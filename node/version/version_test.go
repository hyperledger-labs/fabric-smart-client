/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package version

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetInfo(t *testing.T) {
	t.Parallel()
	info := GetInfo()
	require.Contains(t, info, ProgramName)
	require.Contains(t, info, runtime.Version())
	require.Contains(t, info, runtime.GOOS)
	require.Contains(t, info, runtime.GOARCH)
}

func TestCmd_TrailingArgs(t *testing.T) { //nolint:paralleltest
	cmd := Cmd()
	cmd.SetArgs([]string{"unexpected"})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing args detected")
}

func TestCmd_NoArgs(t *testing.T) { //nolint:paralleltest
	cmd := Cmd()
	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())
}
