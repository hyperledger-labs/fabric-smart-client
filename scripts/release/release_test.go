/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newRepo returns a git repository with the given files in one commit.
func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main")
	run(t, dir, "config", "user.email", "test@example.com")
	run(t, dir, "config", "user.name", "test")
	run(t, dir, "config", "commit.gpgsign", "false")
	run(t, dir, "config", "tag.gpgsign", "false")
	commit(t, dir, files)
	return dir
}

// commit writes files and commits them, an empty commit when there are none.
func commit(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	writeFiles(t, dir, files)
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "--allow-empty", "-m", "commit")
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoError(t, err, string(out))
}

func mustVersion(t *testing.T, s string) semver {
	t.Helper()
	v, ok := parseVersion(s)
	require.True(t, ok, s)
	return v
}

func TestParseVersion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in string
		ok bool
	}{
		{"v0.28.0", true},
		{"v1.10.3", true},
		{"0.28.0", false},
		{"v0.28", false},
		{"v0.28.0-rc.1", false},
		{"integration/v0.28.0", false},
	} {
		v, ok := parseVersion(tc.in)
		require.Equal(t, tc.ok, ok, tc.in)
		if ok {
			require.Equal(t, tc.in, v.String())
		}
	}
}

func TestBump(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		tags []string
		want string
		err  string
	}{
		{name: "next minor", tags: []string{"v0.27.0", "v0.28.0"}, want: "v0.29.0"},
		{name: "after a patch", tags: []string{"v0.28.0", "v0.28.1"}, want: "v0.29.0"},
		{name: "numeric order", tags: []string{"v0.10.0", "v0.9.0"}, want: "v0.11.0"},
		{name: "no tag", err: "no release tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var tags []semver
			for _, s := range tc.tags {
				tags = append(tags, mustVersion(t, s))
			}
			got, err := nextMinor(tags)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got.String())
		})
	}
}

func TestPreviousRelease(t *testing.T) {
	t.Parallel()
	dir := newRepo(t, nil)
	for _, tag := range []string{"v0.28.0", "v0.28.1", "v0.29.0", "integration/v0.30.0", "v0.30.0-rc.1"} {
		run(t, dir, "tag", tag)
	}
	tags, err := releaseTags(dir)
	require.NoError(t, err)
	require.Len(t, tags, 3, "module and pre-release tags are ignored")

	for _, tc := range []struct{ version, want, err string }{
		{version: "v0.30.0", want: "v0.29.0"},
		{version: "v0.29.1", want: "v0.29.0"},
		{version: "v0.28.1", want: "v0.28.0"},
		{version: "v0.28.0", err: "no release before v0.28.0"},
	} {
		got, err := previousRelease(tags, mustVersion(t, tc.version))
		if tc.err != "" {
			require.ErrorContains(t, err, tc.err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, tc.want, got.String(), tc.version)
	}
}

func TestModules(t *testing.T) {
	t.Parallel()
	dir := newRepo(t, map[string]string{
		"go.mod":                        "module root\n",
		"integration/go.mod":            "module integration\n",
		"platform/x/host/libp2p/go.mod": "module libp2p\n",
		"tools/go.mod":                  "module tools\n",
	})
	// A staged go.mod is not in HEAD, so tag-release.sh does not tag it.
	writeFiles(t, dir, map[string]string{"staged/go.mod": "module staged\n"})
	run(t, dir, "add", "staged/go.mod")
	got, err := modules(dir, mustVersion(t, "v0.29.0"))
	require.NoError(t, err)
	require.Equal(t, []string{"integration/v0.29.0", "platform/x/host/libp2p/v0.29.0", "v0.29.0"}, got)
}

func TestShowAtFailsOnAMissingTag(t *testing.T) {
	t.Parallel()
	dir := newRepo(t, map[string]string{"a.txt": "a\n"})
	run(t, dir, "tag", "v0.1.0")
	got, ok, err := showAt(dir, mustVersion(t, "v0.1.0"), "a.txt")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "a\n", string(got))
	_, ok, err = showAt(dir, mustVersion(t, "v0.1.0"), "b.txt")
	require.NoError(t, err)
	require.False(t, ok)
	_, _, err = showAt(dir, mustVersion(t, "v9.9.9"), "a.txt")
	require.Error(t, err, "an unreadable tag is an error, not a missing file")
}

var update = flag.Bool("update", false, "rewrite the golden files under testdata")

// golden compares got with testdata/name, which -update rewrites.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		require.NoError(t, os.WriteFile(path, got, 0o600))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), string(got))
}

const (
	testGoMod    = "module github.com/hyperledger-labs/fabric-smart-client\n\ngo 1.27.3\n"
	testMakefile = `FABRIC_VERSION ?= 3.1.4
FABRIC_X_TOOLS_VERSION ?= v1.0.3
FABRIC_X_COMMITTER_VERSION ?= 1.0.7
POSTGRES_VERSION ?= 16.2-alpine
JAEGER_VERSION ?= 2.12.0
GOLANGCI_LINT_VERSION ?= v2.14.0

postgres:
	docker pull postgres:$(POSTGRES_VERSION)
`
)

// releaseRepo returns a repository released as v0.28.0, with the files of a root module, a
// submodule and the tools module.
func releaseRepo(t *testing.T) string {
	t.Helper()
	dir := newRepo(t, map[string]string{
		"go.mod":             testGoMod,
		"Makefile":           testMakefile,
		"integration/go.mod": "module integration\n",
		"tools/go.mod":       "module tools\n",
	})
	run(t, dir, "tag", "v0.28.0")
	return dir
}

func readReport(t *testing.T) Report {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "report.json"))
	require.NoError(t, err)
	var r Report
	require.NoError(t, json.Unmarshal(raw, &r))
	return r
}

func TestTestedWith(t *testing.T) {
	t.Parallel()
	got, err := testedWith([]byte(testGoMod), []byte(testMakefile))
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"fabric":             "3.1.4",
		"fabric_x_committer": "1.0.7",
		"fabric_x_tools":     "v1.0.3",
		"go":                 "1.27.3",
		"postgres":           "16.2",
	}, got)

	_, err = testedWith([]byte(testGoMod), []byte("FABRIC_VERSION ?= 3.1.4\n"))
	require.ErrorContains(t, err, "POSTGRES_VERSION")
	_, err = testedWith([]byte("module x\n"), []byte(testMakefile))
	require.ErrorContains(t, err, "go directive")
}

func TestDataFromReport(t *testing.T) {
	t.Parallel()
	dir := releaseRepo(t)
	d, err := newData(dir, mustVersion(t, "v0.29.0"), readReport(t))
	require.NoError(t, err)
	raw, err := d.render()
	require.NoError(t, err)
	golden(t, "v0.29.0.json", raw)

	_, err = newData(dir, mustVersion(t, "v0.28.0"), readReport(t))
	require.ErrorContains(t, err, "no release before v0.28.0")

	_, err = newData(dir, mustVersion(t, "v0.28.1"), readReport(t))
	require.ErrorContains(t, err, "patch releases are not supported")
}

func TestRenderIsDeterministic(t *testing.T) {
	t.Parallel()
	dir := releaseRepo(t)
	var out [][]byte
	for range 2 {
		d, err := newData(dir, mustVersion(t, "v0.29.0"), readReport(t))
		require.NoError(t, err)
		raw, err := d.render()
		require.NoError(t, err)
		out = append(out, raw)
	}
	require.Equal(t, out[0], out[1])
	require.True(t, bytes.HasSuffix(out[0], []byte("}\n")))
}

func TestNotes(t *testing.T) {
	t.Parallel()
	dir := releaseRepo(t)
	d, err := newData(dir, mustVersion(t, "v0.29.0"), readReport(t))
	require.NoError(t, err)
	changelog, err := os.ReadFile(filepath.Join("testdata", "changelog.md"))
	require.NoError(t, err)

	// A minor needs its upgrade guide, which the notes link to.
	_, err = notes(dir, d, string(changelog))
	require.ErrorContains(t, err, "docs/upgrade/v0.28-to-v0.29.md")

	writeFiles(t, dir, map[string]string{d.Upgrade.Guide: "# Upgrading from v0.28 to v0.29\n"})
	got, err := notes(dir, d, string(changelog))
	require.NoError(t, err)
	golden(t, "v0.29.0.md", []byte(got))

	// The notes link to the guide on main, so a fix to the guide reaches every reader, and to the
	// release data at the tag, which never changes. They copy neither.
	const blob = "https://github.com/hyperledger-labs/fabric-smart-client/blob/"
	require.Contains(t, got, "[upgrade guide]("+blob+"main/docs/upgrade/v0.28-to-v0.29.md)")
	require.Contains(t, got, "]("+blob+"v0.29.0/docs/upgrade/releases/v0.29.0.json)")
	for _, s := range []string{"## Checklist", "## Schema steps", "## Protocol versions", "Stop-all", "#checklist"} {
		require.NotContains(t, got, s)
	}
	// Tested with is one row, in the order of testedNames.
	require.Contains(t, got, "| Go | Hyperledger Fabric | Fabric-x committer | Fabric-x tools | PostgreSQL |\n|---|---|---|---|---|\n| 1.27.3 | 3.1.4 | 1.0.7 | v1.0.3 | 16.2 |\n")
	// The changelog follows unchanged.
	require.True(t, strings.HasSuffix(got, strings.TrimSpace(string(changelog))+"\n"))
}

func TestCheck(t *testing.T) {
	t.Parallel()
	r := readReport(t)
	dir := releaseRepo(t)
	require.NoError(t, check(dir, r), "no data files")

	// v0.28.0 was cut before the tooling: its tag has no data file, so its file is not checked.
	writeFiles(t, dir, map[string]string{"docs/upgrade/releases/v0.28.0.json": "{}\n"})
	require.NoError(t, check(dir, r))

	// A release PR: the untagged file must be what data generates.
	d, err := newData(dir, mustVersion(t, "v0.29.0"), r)
	require.NoError(t, err)
	raw, err := d.render()
	require.NoError(t, err)
	writeFiles(t, dir, map[string]string{"docs/upgrade/releases/v0.29.0.json": string(raw)})
	require.NoError(t, check(dir, r))
	writeFiles(t, dir, map[string]string{"docs/upgrade/releases/v0.29.0.json": strings.Replace(string(raw), "stop-all", "rolling", 1)})
	require.ErrorContains(t, check(dir, r), `docs/upgrade/releases/v0.29.0.json is not what "data" generates`)

	// The same error names the other cause: an untouched file goes stale when main moves on,
	// here by a component range.
	writeFiles(t, dir, map[string]string{"docs/upgrade/releases/v0.29.0.json": string(raw)})
	moved := readReport(t)
	moved.Components[0].Max++
	require.ErrorContains(t, check(dir, moved), "or the tree changed since it was generated")

	// A released file never changes.
	writeFiles(t, dir, map[string]string{"docs/upgrade/releases/v0.29.0.json": string(raw)})
	commit(t, dir, nil)
	run(t, dir, "tag", "v0.29.0")
	require.NoError(t, check(dir, r))
	writeFiles(t, dir, map[string]string{"docs/upgrade/releases/v0.29.0.json": strings.Replace(string(raw), "stop-all", "rolling", 1)})
	require.ErrorContains(t, check(dir, r), "docs/upgrade/releases/v0.29.0.json differs from v0.29.0")
}

// TestRefnodeReportsEveryGoldenStore fails when a store has golden steps but the reference binary
// does not report it, so its steps would be missing from the release data.
// stepsDirs hold the golden steps files, <dialect>-<store>.steps, of every FSC store.
var stepsDirs = []string{
	"platform/view/services/storage/driver/sql/testdata/steps",
	"platform/fabric/services/db/driver/sql/testdata/steps",
}

func TestRefnodeReportsEveryGoldenStore(t *testing.T) {
	t.Parallel()
	out, err := exec.Command("go", "run", "./refnode", "version", "--json").Output()
	require.NoError(t, err)
	var r Report
	require.NoError(t, json.Unmarshal(out, &r))
	reported := map[string]bool{}
	for _, s := range r.Stores {
		reported[s.Dialect+"-"+s.Type] = true
	}
	for _, d := range stepsDirs {
		files, err := filepath.Glob(filepath.Join("..", "..", filepath.FromSlash(d), "*.steps"))
		require.NoError(t, err)
		require.NotEmpty(t, files, d)
		for _, f := range files {
			name := strings.TrimSuffix(filepath.Base(f), ".steps")
			require.True(t, reported[name], "refnode does not report the store of %s", f)
		}
	}
}
