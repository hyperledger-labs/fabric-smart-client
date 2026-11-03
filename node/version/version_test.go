/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package version

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/buildinfo"
)

func TestGetInfo(t *testing.T) {
	t.Parallel()
	info := GetInfo()
	require.Contains(t, info, ProgramName)
	require.Contains(t, info, runtime.Version())
	require.Contains(t, info, runtime.GOOS)
	require.Contains(t, info, runtime.GOARCH)
}

func TestCmd_TrailingArgs(t *testing.T) {
	t.Parallel()

	cmd := Cmd()
	cmd.SetArgs([]string{"unexpected"})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing args detected")
}

func TestCmd_NoArgs(t *testing.T) {
	t.Parallel()

	cmd := Cmd()
	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())
}

func TestFormat(t *testing.T) {
	t.Parallel()
	prefix := ProgramName + ":\n Go version: " + runtime.Version() + "\n OS/Arch: " + runtime.GOOS + "/" + runtime.GOARCH + "\n"

	got := format(buildinfo.Info{FSCVersion: "v0.27.0", AppModule: "github.com/acme/app", AppVersion: "v1.4.0", Revision: "abc123", Modified: true})
	require.Equal(t, prefix+" FSC version: v0.27.0\n Application: github.com/acme/app v1.4.0\n Revision: abc123 (modified)\n", got)

	got = format(buildinfo.Info{FSCVersion: "(devel)"})
	require.True(t, strings.HasPrefix(got, prefix), got)
	require.Contains(t, got, " FSC version: (devel)\n")
	require.Contains(t, got, " Application: unknown\n")
	require.Contains(t, got, " Revision: unknown\n")
}
