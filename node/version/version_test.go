/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package version

import (
	"runtime"
	"runtime/debug"
	"strings"
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

func TestFromBuildInfo(t *testing.T) {
	t.Parallel()
	app := debug.Module{Path: "github.com/acme/app", Version: "v1.4.0"}
	vcs := []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}, {Key: "vcs.modified", Value: "true"}}
	tests := []struct {
		name string
		bi   *debug.BuildInfo
		ok   bool
		want buildInfo
	}{
		{
			name: "dependency",
			bi:   &debug.BuildInfo{Main: app, Deps: []*debug.Module{{Path: fscModule, Version: "v0.27.0"}}, Settings: vcs},
			ok:   true,
			want: buildInfo{fscVersion: "v0.27.0", appModule: app.Path, appVersion: app.Version, revision: "abc123", modified: true},
		},
		{
			name: "dependency replaced by a local directory",
			bi:   &debug.BuildInfo{Main: app, Deps: []*debug.Module{{Path: fscModule, Version: "v0.27.0", Replace: &debug.Module{Path: "../fsc"}}}},
			ok:   true,
			want: buildInfo{fscVersion: "(devel)", appModule: app.Path, appVersion: app.Version},
		},
		{
			name: "dependency replaced by a fork",
			bi:   &debug.BuildInfo{Main: app, Deps: []*debug.Module{{Path: fscModule, Version: "v0.27.0", Replace: &debug.Module{Path: "github.com/fork/fsc", Version: "v0.27.1"}}}},
			ok:   true,
			want: buildInfo{fscVersion: "v0.27.1", appModule: app.Path, appVersion: app.Version},
		},
		{
			name: "main module",
			bi:   &debug.BuildInfo{Main: debug.Module{Path: fscModule, Version: "v0.27.0"}},
			ok:   true,
			want: buildInfo{fscVersion: "v0.27.0", appModule: fscModule, appVersion: "v0.27.0"},
		},
		{
			name: "main module without a version",
			bi:   &debug.BuildInfo{Main: debug.Module{Path: fscModule}},
			ok:   true,
			want: buildInfo{fscVersion: "(devel)", appModule: fscModule},
		},
		{
			name: "missing",
			want: buildInfo{fscVersion: "(devel)"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, fromBuildInfo(tc.bi, tc.ok))
		})
	}
}

func TestFormat(t *testing.T) {
	t.Parallel()
	prefix := ProgramName + ":\n Go version: " + runtime.Version() + "\n OS/Arch: " + runtime.GOOS + "/" + runtime.GOARCH + "\n"

	got := format(buildInfo{fscVersion: "v0.27.0", appModule: "github.com/acme/app", appVersion: "v1.4.0", revision: "abc123", modified: true})
	require.Equal(t, prefix+" FSC version: v0.27.0\n Application: github.com/acme/app v1.4.0\n Revision: abc123 (modified)\n", got)

	got = format(buildInfo{fscVersion: "(devel)"})
	require.True(t, strings.HasPrefix(got, prefix), got)
	require.Contains(t, got, " FSC version: (devel)\n")
	require.Contains(t, got, " Application: unknown\n")
	require.Contains(t, got, " Revision: unknown\n")
}
