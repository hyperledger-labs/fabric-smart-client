/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package buildinfo

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFromBuildInfo(t *testing.T) {
	t.Parallel()
	app := debug.Module{Path: "github.com/acme/app", Version: "v1.4.0"}
	vcs := []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}, {Key: "vcs.modified", Value: "true"}}
	tests := []struct {
		name string
		bi   *debug.BuildInfo
		ok   bool
		want Info
	}{
		{
			name: "dependency",
			bi:   &debug.BuildInfo{Main: app, Deps: []*debug.Module{{Path: FSCModule, Version: "v0.27.0"}}, Settings: vcs},
			ok:   true,
			want: Info{FSCVersion: "v0.27.0", AppModule: app.Path, AppVersion: app.Version, Revision: "abc123", Modified: true},
		},
		{
			name: "dependency replaced by a local directory",
			bi:   &debug.BuildInfo{Main: app, Deps: []*debug.Module{{Path: FSCModule, Version: "v0.27.0", Replace: &debug.Module{Path: "../fsc"}}}},
			ok:   true,
			want: Info{FSCVersion: "(devel)", AppModule: app.Path, AppVersion: app.Version},
		},
		{
			name: "dependency replaced by a fork",
			bi:   &debug.BuildInfo{Main: app, Deps: []*debug.Module{{Path: FSCModule, Version: "v0.27.0", Replace: &debug.Module{Path: "github.com/fork/fsc", Version: "v0.27.1"}}}},
			ok:   true,
			want: Info{FSCVersion: "v0.27.1", AppModule: app.Path, AppVersion: app.Version},
		},
		{
			name: "main module",
			bi:   &debug.BuildInfo{Main: debug.Module{Path: FSCModule, Version: "v0.27.0"}},
			ok:   true,
			want: Info{FSCVersion: "v0.27.0", AppModule: FSCModule, AppVersion: "v0.27.0"},
		},
		{
			name: "main module without a version",
			bi:   &debug.BuildInfo{Main: debug.Module{Path: FSCModule}},
			ok:   true,
			want: Info{FSCVersion: "(devel)", AppModule: FSCModule},
		},
		{
			name: "missing",
			want: Info{FSCVersion: "(devel)"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, FromBuildInfo(tc.bi, tc.ok))
		})
	}
}
