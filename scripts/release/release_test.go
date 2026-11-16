/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
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

func TestPreviousCommand(t *testing.T) {
	t.Parallel()
	dir := newRepo(t, nil)
	for _, tag := range []string{"v0.27.0", "v0.28.0", "integration/v0.29.0"} {
		run(t, dir, "tag", tag)
	}
	out, err := runCommand(dir, "previous", []string{"--version", "v0.29.0"})
	require.NoError(t, err)
	require.Equal(t, "v0.28.0\n", out)
	_, err = runCommand(dir, "previous", []string{"--version", "v0.27.0"})
	require.ErrorContains(t, err, "no release before v0.27.0")
	_, err = runCommand(dir, "previous", nil)
	require.ErrorContains(t, err, "previous needs --version")
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
	for _, tag := range []string{"v0.28.0", "v0.28.1", "v0.29.0", "v0.29.1", "v0.29.2", "integration/v0.30.0", "v0.30.0-rc.1"} {
		run(t, dir, "tag", tag)
	}
	tags, err := releaseTags(dir)
	require.NoError(t, err)
	require.Len(t, tags, 5, "module and pre-release tags are ignored")

	for _, tc := range []struct{ version, want, err string }{
		// A minor follows the previous minor: patches are on a release branch, not on main.
		{version: "v0.30.0", want: "v0.29.0"},
		{version: "v0.31.0", err: "no release v0.30.0 before v0.31.0"},
		// A patch follows the previous patch of its minor, or the minor itself.
		{version: "v0.29.3", want: "v0.29.2"},
		{version: "v0.29.1", want: "v0.29.0"},
		{version: "v0.28.2", want: "v0.28.1"},
		{version: "v0.28.3", err: "no release v0.28.2 before v0.28.3"},
		{version: "v0.30.1", err: "no release v0.30.0 before v0.30.1"},
		{version: "v0.28.0", err: "no release before v0.28.0"},
		{version: "v1.0.0", want: "v0.29.0"},
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
// submodule and the tools module, and release data of v0.28.0 with the stores and components of
// testdata/report.json.
func releaseRepo(t *testing.T) string {
	t.Helper()
	report, err := os.ReadFile(filepath.Join("testdata", "report.json"))
	require.NoError(t, err)
	dir := newRepo(t, map[string]string{
		"go.mod":                      testGoMod,
		"Makefile":                    testMakefile,
		"integration/go.mod":          "module integration\n",
		"tools/go.mod":                "module tools\n",
		releasesDir + "/v0.28.0.json": string(report),
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

// TestTestedWithDroppedRow drops PostgreSQL from the table: its key leaves tested_with.
func TestTestedWithDroppedRow(t *testing.T) { //nolint:paralleltest // changes the table tested
	saved := tested
	t.Cleanup(func() { tested = saved })
	tested = nil
	for _, c := range saved {
		if c.key != "postgres" {
			tested = append(tested, c)
		}
	}
	got, err := testedWith([]byte(testGoMod), []byte(testMakefile))
	require.NoError(t, err)
	require.NotContains(t, got, "postgres")
	require.Len(t, got, len(tested))
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

	// A patch rolls out over its minor and has no upgrade guide of its own.
	d, err = newData(dir, mustVersion(t, "v0.28.1"), readReport(t))
	require.NoError(t, err)
	require.Equal(t, "v0.28.1", d.Version)
	require.Equal(t, Upgrade{Mode: "rolling"}, d.Upgrade)
	require.Contains(t, d.Modules, "integration/v0.28.1")

	_, err = newData(dir, mustVersion(t, "v0.28.2"), readReport(t))
	require.ErrorContains(t, err, "no release v0.28.1 before v0.28.2")
}

func TestPatchKeepsSchemaAndWireFormat(t *testing.T) {
	t.Parallel()
	dir := releaseRepo(t)
	_, err := newData(dir, mustVersion(t, "v0.28.1"), readReport(t))
	require.NoError(t, err, "the same stores and components")

	moved := readReport(t)
	moved.Components[0].Max++
	_, err = newData(dir, mustVersion(t, "v0.28.1"), moved)
	require.ErrorContains(t, err, "v0.28.1 changes the schema steps or component ranges of v0.28.0")
	moved = readReport(t)
	moved.Stores[0].Steps = append(moved.Stores[0].Steps, Step{Version: 99, Name: "new"})
	_, err = newData(dir, mustVersion(t, "v0.28.1"), moved)
	require.ErrorContains(t, err, "a patch cannot")

	// Without the release data of its minor, a patch cannot show that it rolls out.
	old := newRepo(t, map[string]string{"go.mod": testGoMod, "Makefile": testMakefile})
	run(t, old, "tag", "v0.27.0")
	_, err = newData(old, mustVersion(t, "v0.27.1"), readReport(t))
	require.ErrorContains(t, err, "v0.27.0 has no release data")
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
	_, err = notes(dir, mainRepo, d, string(changelog))
	require.ErrorContains(t, err, "docs/upgrade/v0.28-to-v0.29.md")

	writeFiles(t, dir, map[string]string{d.Upgrade.Guide: "# Upgrading from v0.28 to v0.29\n"})
	got, err := notes(dir, mainRepo, d, string(changelog))
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
	// A release from a fork links to the fork.
	fork, err := notes(dir, "someone/fabric-smart-client", d, string(changelog))
	require.NoError(t, err)
	require.Contains(t, fork, "(https://github.com/someone/fabric-smart-client/blob/main/docs/upgrade/v0.28-to-v0.29.md)")
	require.Contains(t, fork, "(https://github.com/someone/fabric-smart-client/blob/v0.29.0/docs/upgrade/releases/v0.29.0.json)")
	// Tested with is one row, in the order of tested.
	require.Contains(t, got, "| Go | Hyperledger Fabric | Chaguar committer | Chaguar tools | PostgreSQL |\n|---|---|---|---|---|\n| 1.27.3 | 3.1.4 | 1.0.7 | v1.0.3 | 16.2 |\n")
	// The changelog follows unchanged.
	require.True(t, strings.HasSuffix(got, strings.TrimSpace(string(changelog))+"\n"))

	// A minor without a guide is refused, not linked to the repository root.
	d.Upgrade.Guide = ""
	_, err = notes(dir, mainRepo, d, string(changelog))
	require.ErrorContains(t, err, "v0.29.0 is a minor and has no upgrade guide")
}

func TestPatchNotes(t *testing.T) {
	t.Parallel()
	dir := releaseRepo(t)
	d, err := newData(dir, mustVersion(t, "v0.28.1"), readReport(t))
	require.NoError(t, err)
	got, err := notes(dir, mainRepo, d, "## What's Changed\n* fix: something\n")
	require.NoError(t, err)
	golden(t, "v0.28.1.md", []byte(got))
	require.Contains(t, got, "No breaking changes. Upgrade from v0.28.x at any time")
	require.NotContains(t, got, "upgrade guide")
	require.Contains(t, got, "](https://github.com/hyperledger-labs/fabric-smart-client/blob/v0.28.1/docs/upgrade/releases/v0.28.1.json)")
}

func TestCheck(t *testing.T) {
	t.Parallel()
	r := readReport(t)
	dir := releaseRepo(t)
	require.NoError(t, check(dir, r), "no data files")

	// A release cut before the tooling: its tag has no data file, so its file is not checked.
	old := newRepo(t, nil)
	run(t, old, "tag", "v0.27.0")
	writeFiles(t, old, map[string]string{"docs/upgrade/releases/v0.27.0.json": "{}\n"})
	require.NoError(t, check(old, r))

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

// TestMergify checks the backport rules of each release: the two highest minors from v0.29.0
// on, whatever the patches.
func TestMergify(t *testing.T) {
	t.Parallel()
	dir := releaseRepo(t)
	rules := func() []string {
		t.Helper()
		out, err := mergify(dir)
		require.NoError(t, err)
		var got []string
		for line := range strings.Lines(out) {
			if b, ok := strings.CutPrefix(strings.TrimSpace(line), "branches: "); ok {
				got = append(got, b)
			}
		}
		return got
	}
	require.Empty(t, rules(), "v0.28.0 was cut before patch releases")
	for _, tc := range []struct {
		version string
		want    []string
	}{
		{"v0.29.0", []string{"[release-0.29]"}},
		{"v0.29.1", []string{"[release-0.29]"}},
		{"v0.30.0", []string{"[release-0.30]", "[release-0.29]"}},
		{"v0.31.0", []string{"[release-0.31]", "[release-0.30]"}},
	} {
		writeFiles(t, dir, map[string]string{releasesDir + "/" + tc.version + ".json": "{}\n"})
		require.Equal(t, tc.want, rules(), tc.version)
	}
	out, err := mergify(dir)
	require.NoError(t, err)
	golden(t, "mergify-v0.31.0.yml", []byte(out))
}

func TestCheckMergify(t *testing.T) {
	t.Parallel()
	dir := releaseRepo(t)
	require.ErrorContains(t, checkMergify(dir), "failed reading .mergify.yml")
	want, err := mergify(dir)
	require.NoError(t, err)
	writeFiles(t, dir, map[string]string{mergifyFile: want})
	require.NoError(t, checkMergify(dir))

	// A new minor's release data needs its rule.
	writeFiles(t, dir, map[string]string{releasesDir + "/v0.29.0.json": "{}\n"})
	require.ErrorContains(t, checkMergify(dir), "came without its rule")

	writeFiles(t, dir, map[string]string{mergifyFile: want + "  - name: a hand-written rule\n"})
	require.ErrorContains(t, checkMergify(dir), "it was edited")
}

func TestAPIDiff(t *testing.T) {
	t.Parallel()
	const gen = "// Code generated by counterfeiter. DO NOT EDIT.\n\n"
	// The API at v0.1.0: a root module and a submodule, and a tools module that is not
	// released.
	files := map[string]string{
		"go.mod":            "module example.com/m\n\ngo 1.22\n",
		"a/a.go":            "package a\n\nfunc F() {}\n\ntype I interface{ M() }\n\n// S is not for applications to implement.\ntype S interface {\n\tN()\n\tsealed()\n}\n",
		"gone/gone.go":      "package gone\n\nfunc G() {}\n",
		"a/mock/i.go":       gen + "package mock\n\ntype I struct{}\n\nfunc (I) M() {}\n",
		"a/fake/i.go":       "package fake\n\ntype I struct{}\n\nfunc (I) M() {}\n",
		"hsm/hsm.go":        "//go:build pkcs11\n\npackage hsm\n\nfunc Open(string) {}\n",
		"hsm/disabled.go":   "//go:build !pkcs11\n\npackage hsm\n",
		"sub/go.mod":        "module example.com/m/sub\n\ngo 1.22\n",
		"sub/sub.go":        "package sub\n\nfunc H() {}\n",
		"tools/go.mod":      "module tools\n\ngo 1.22\n",
		"tools/tools.go":    "package tools\n\nfunc T() {}\n",
		"a/mock/i_test.go":  "package mock\n",
		"internal/x/x.go":   "package x\n\nfunc X() {}\n",
		"a/mock/README.txt": "not Go\n",
	}
	for _, tc := range []struct {
		name   string
		change map[string]string
		remove string
		want   []string // the lines of the error; none for a compatible change
	}{
		{name: "an added function", change: map[string]string{"a/new.go": "package a\n\nfunc New() {}\n"}},
		{name: "an added package", change: map[string]string{"b/b.go": "package b\n\nfunc B() {}\n"}},
		{name: "a method added to an interface with an unexported method", change: map[string]string{
			"a/a.go": "package a\n\nfunc F() {}\n\ntype I interface{ M() }\n\ntype S interface {\n\tN()\n\tO()\n\tsealed()\n}\n",
		}},
		{name: "a changed generated mock", change: map[string]string{"a/mock/i.go": gen + "package mock\n\ntype J struct{}\n"}},
		{name: "a changed internal package", change: map[string]string{"internal/x/x.go": "package x\n"}},
		{name: "a changed tools module", change: map[string]string{"tools/tools.go": "package tools\n"}},
		{
			name: "a removed function", change: map[string]string{"gone/gone.go": "package gone\n"},
			want: []string{"example.com/m:", "- ./gone.G: removed (default, pkcs11)"},
		},
		{
			name: "a changed signature", change: map[string]string{"sub/sub.go": "package sub\n\nfunc H(int) {}\n"},
			want: []string{"example.com/m/sub:", "- H: changed from func() to func(int) (default, pkcs11)"},
		},
		{name: "a method added to an interface", change: map[string]string{
			"a/a.go": "package a\n\nfunc F() {}\n\ntype I interface{ M(); N() }\n\ntype S interface {\n\tN()\n\tsealed()\n}\n",
		}, want: []string{"example.com/m:", "- ./a.I.N: added (default, pkcs11)"}},
		{
			name: "a removed package", remove: "gone/gone.go",
			want: []string{"example.com/m:", "- package example.com/m/gone: removed (default, pkcs11)"},
		},
		{
			name: "a changed hand-written fake", change: map[string]string{"a/fake/i.go": "package fake\n\ntype I struct{}\n"},
			want: []string{"example.com/m:", "- ./a/fake.I.M: removed (default, pkcs11)"},
		},
		{
			name: "a change only the pkcs11 build has", change: map[string]string{"hsm/hsm.go": "//go:build pkcs11\n\npackage hsm\n\nfunc Open(string, int) {}\n"},
			want: []string{"example.com/m:", "- ./hsm.Open: changed from func(string) to func(string, int) (pkcs11)"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := newRepo(t, files)
			run(t, dir, "tag", "v0.1.0")
			if tc.remove != "" {
				run(t, dir, "rm", "-q", tc.remove)
			}
			commit(t, dir, tc.change)
			// An untracked change is not part of HEAD and is not checked.
			writeFiles(t, dir, map[string]string{"a/a.go": "package a\n"})

			_, err := runCommand(dir, "apidiff", []string{"--base", "v0.1.0"})
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			msg := err.Error()
			require.Contains(t, msg, fmt.Sprintf("HEAD has %d incompatible API changes since v0.1.0", len(tc.want)-1))
			_, list, _ := strings.Cut(msg, "\n\n")
			require.Equal(t, tc.want, strings.Split(strings.TrimSpace(list), "\n"))
		})
	}
}

// TestAPIDiffKeepsItsBuildTags checks that the caller's build tags change neither pass. It sets
// GOFLAGS, so it does not run in parallel.
func TestAPIDiffKeepsItsBuildTags(t *testing.T) {
	t.Setenv("GOFLAGS", "-tags=pkcs11 -modcacherw")
	dir := newRepo(t, map[string]string{
		"go.mod":          "module example.com/m\n\ngo 1.22\n",
		"hsm/hsm.go":      "//go:build pkcs11\n\npackage hsm\n\nfunc Open(string) {}\n",
		"hsm/disabled.go": "//go:build !pkcs11\n\npackage hsm\n",
	})
	run(t, dir, "tag", "v0.1.0")
	commit(t, dir, map[string]string{"hsm/hsm.go": "//go:build pkcs11\n\npackage hsm\n"})
	_, err := runCommand(dir, "apidiff", []string{"--base", "v0.1.0"})
	require.ErrorContains(t, err, "- ./hsm.Open: removed (pkcs11)\n")
}

func TestChangedPackage(t *testing.T) {
	t.Parallel()
	for change, want := range map[string]string{
		"package example.com/m/a/b: removed":             "example.com/m/a/b",
		"./a/b.F: removed":                               "example.com/m/a/b",
		"./a/b.(*T).M: changed from func(x.Y) to func()": "example.com/m/a/b",
		"./a.T.f: changed from int to string":            "example.com/m/a",
	} {
		require.Equal(t, want, changedPackage(change, "example.com/m"), change)
	}
}
