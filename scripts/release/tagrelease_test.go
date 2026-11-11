/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// tagRepo returns a repository with a root module and a submodule, a copy of
// scripts/tag-release.sh, and a bare repository as its origin. With data, the commit holds the
// release data file of v0.29.0.
func tagRepo(t *testing.T, data bool) (dir, origin string) {
	t.Helper()
	script, err := os.ReadFile(filepath.Join("..", "tag-release.sh"))
	require.NoError(t, err)
	files := map[string]string{"go.mod": "module x\n", "sub/go.mod": "module x/sub\n"}
	if data {
		files[releasesDir+"/v0.29.0.json"] = "{}\n"
	}
	dir = newRepo(t, files)
	// The script finds the repository from its own path.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "scripts"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scripts", "tag-release.sh"), script, 0o700)) //nolint:gosec // an executable script
	origin = t.TempDir()
	run(t, origin, "init", "-q", "--bare")
	run(t, dir, "remote", "add", "origin", origin)
	return dir, origin
}

func tagRelease(dir string, args ...string) (string, error) {
	out, err := exec.Command(filepath.Join(dir, "scripts", "tag-release.sh"), args...).CombinedOutput() //nolint:gosec // a test script
	return string(out), err
}

func tags(t *testing.T, dir string) []string {
	t.Helper()
	out, err := git(dir, "tag", "--list")
	require.NoError(t, err)
	return strings.Fields(out)
}

func TestTagRelease(t *testing.T) {
	t.Parallel()

	t.Run("creates one tag per module", func(t *testing.T) {
		t.Parallel()
		dir, origin := tagRepo(t, true)
		out, err := tagRelease(dir, "v0.29.0")
		require.NoError(t, err, out)
		require.ElementsMatch(t, []string{"v0.29.0", "sub/v0.29.0"}, tags(t, dir))
		require.Empty(t, tags(t, origin), "pushed without --push")
		require.Contains(t, out, "git push upstream")
	})

	t.Run("refuses a commit without the release data", func(t *testing.T) {
		t.Parallel()
		dir, _ := tagRepo(t, false)
		// A file only in the working tree is not in the tagged commit.
		writeFiles(t, dir, map[string]string{releasesDir + "/v0.29.0.json": "{}\n"})
		out, err := tagRelease(dir, "v0.29.0")
		require.Error(t, err)
		require.Contains(t, out, releasesDir+"/v0.29.0.json")
		require.Empty(t, tags(t, dir))
	})

	t.Run("refuses a patch without its release data", func(t *testing.T) {
		t.Parallel()
		dir, _ := tagRepo(t, true)
		out, err := tagRelease(dir, "v0.29.1")
		require.Error(t, err)
		require.Contains(t, out, releasesDir+"/v0.29.1.json")
		require.Empty(t, tags(t, dir))
	})

	t.Run("refuses an existing tag and creates none", func(t *testing.T) {
		t.Parallel()
		dir, _ := tagRepo(t, true)
		run(t, dir, "tag", "sub/v0.29.0")
		out, err := tagRelease(dir, "v0.29.0")
		require.Error(t, err)
		require.Contains(t, out, "sub/v0.29.0")
		require.Equal(t, []string{"sub/v0.29.0"}, tags(t, dir))
	})

	t.Run("pushes exactly the new tags", func(t *testing.T) {
		t.Parallel()
		dir, origin := tagRepo(t, true)
		run(t, dir, "tag", "unrelated")
		out, err := tagRelease(dir, "--push=origin", "v0.29.0")
		require.NoError(t, err, out)
		require.ElementsMatch(t, []string{"v0.29.0", "sub/v0.29.0"}, tags(t, origin))
	})
}
