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

// The fakes stand in for the tools scripts/release.sh calls. Each appends its arguments
// to $FAKE_LOG.
var fakes = map[string]string{
	// go run ./scripts/release <cmd> and refnode; everything else (go test) succeeds.
	"go": `echo "go $*" >> "$FAKE_LOG"
case "$*" in
  *"scripts/release bump"*) echo v0.29.0 ;;
  # The highest release tag below --version, as TestPreviousCommand checks the real command.
  *"scripts/release previous"*)
    v=${!#}
    p=$( (git tag --list 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$'; echo "$v") | sort -u -V | grep -B1 -x -- "$v" | head -n1)
    [[ "$p" != "$v" ]] || { echo "no release before $v" >&2; exit 1; }
    echo "$p" ;;
  *"refnode version"*) echo '{}' ;;
  *"scripts/release data"*) echo '{"version":"v0.29.0"}' ;;
  *"scripts/release notes"*) echo "notes ${FAKE_NOTES:-1}" ;;
esac`,
	// gh: generate-notes, the release PR lookups, the Tests run and the release.
	"gh": `echo "gh $*" >> "$FAKE_LOG"
case "$*" in
  "auth status"*) [[ -z "${FAKE_NO_GH:-}" ]] ;;
  "pr list"*"--search"*) echo "${FAKE_OTHER_RELEASE_PR:-}" ;;
  "pr list"*) echo "${FAKE_OPEN_PR:-}" ;;
  "release view"*) [[ -n "${FAKE_RELEASED:-}" ]] ;;
  "run list"*"--event push"*) echo "${FAKE_TESTS-success}" ;;
esac`,
	"docker": `[[ -z "${FAKE_NO_DOCKER:-}" ]]`,
	"make":   `echo "make $*" >> "$FAKE_LOG"`,
	// git commit -S: the status line git looks for from a signer.
	"gpg": `echo "[GNUPG:] SIG_CREATED " >&2; echo signature`,
}

// releaseFixture is a repository like FSC's as far as scripts/release.sh can tell, with
// an upgrade guide for v0.29 and tag v0.28.0, cloned from a bare "upstream".
type releaseFixture struct {
	t                  *testing.T
	dir, upstream, log string
	// target is the bare repository the release goes to: upstream, or the fork.
	target string
	env    []string
}

func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()
	files := map[string]string{
		"go.mod":                         "module x\n",
		"sub/go.mod":                     "module x/sub\n",
		"docs/upgrade/v0.28-to-v0.29.md": "guide\n",
		releasesDir + "/README.md":       "readme\n",
		fixturesDir + "/generate.sh":     "#!/usr/bin/env bash\nmkdir -p \"$(dirname \"$0\")/$2\" && echo fixture > \"$(dirname \"$0\")/$2/SHA256SUMS\"\n",
		"scripts/release.sh":             readScript(t, "release.sh"),
		"scripts/tag-release.sh":         readScript(t, "tag-release.sh"),
	}
	src := newRepo(t, files)
	require.NoError(t, os.Chmod(filepath.Join(src, fixturesDir, "generate.sh"), 0o700))  //nolint:gosec // an executable script
	require.NoError(t, os.Chmod(filepath.Join(src, "scripts", "release.sh"), 0o700))     //nolint:gosec // an executable script
	require.NoError(t, os.Chmod(filepath.Join(src, "scripts", "tag-release.sh"), 0o700)) //nolint:gosec // an executable script
	run(t, src, "add", "-A")
	run(t, src, "commit", "-q", "-m", "executables")
	run(t, src, "tag", "v0.28.0")

	f := &releaseFixture{t: t, upstream: filepath.Join(t.TempDir(), "upstream.git")}
	f.target = f.upstream
	run(t, src, "clone", "-q", "--bare", src, f.upstream)
	f.dir = filepath.Join(t.TempDir(), "fsc")
	run(t, src, "clone", "-q", "-o", "upstream", f.upstream, f.dir)
	for _, c := range [][]string{{"user.email", "test@example.com"}, {"user.name", "test"}, {"tag.gpgsign", "false"}} {
		run(t, f.dir, "config", c[0], c[1])
	}

	bin := t.TempDir()
	for name, body := range fakes {
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/usr/bin/env bash\n"+body+"\n"), 0o700)) //nolint:gosec // an executable fake
	}
	f.log = filepath.Join(t.TempDir(), "log")
	f.env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_LOG="+f.log,
		"RELEASE_REPO_URL=upstream\\.git$",
		"YES=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
	)
	return f
}

func readScript(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", name))
	require.NoError(t, err)
	return string(b)
}

// asFork makes the checkout release to a fork of upstream, its remote origin, with the
// release tags of upstream as the main repository's. The fork has no release tags.
func (f *releaseFixture) asFork() {
	f.t.Helper()
	f.target = filepath.Join(f.t.TempDir(), "fork.git")
	run(f.t, f.dir, "clone", "-q", "--bare", "--no-tags", f.upstream, f.target)
	run(f.t, f.dir, "remote", "add", "origin", f.target)
	f.env = append(f.env,
		"RELEASE_REPO=someone/fabric-smart-client",
		"RELEASE_REPO_URL=fork\\.git$",
		"RELEASE_MAIN_URL="+f.upstream,
	)
}

// release runs scripts/release.sh step with extra environment variables.
func (f *releaseFixture) release(step string, env ...string) (string, error) {
	cmd := exec.Command(filepath.Join(f.dir, "scripts", "release.sh"), step) //nolint:gosec // a test script
	cmd.Dir = f.dir
	cmd.Env = append(append([]string{}, f.env...), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (f *releaseFixture) mustRelease(step string, env ...string) string {
	f.t.Helper()
	out, err := f.release(step, env...)
	require.NoError(f.t, err, out)
	return out
}

// merge squash-merges the pushed release branch of v0.29.0 into main of the target.
func (f *releaseFixture) merge() { f.mergeTo("v0.29.0", "main") }

// mergeTo squash-merges the pushed release branch of version into branch base of the target,
// as GitHub does.
func (f *releaseFixture) mergeTo(version, base string) {
	f.t.Helper()
	work := filepath.Join(f.t.TempDir(), "merge")
	run(f.t, f.dir, "clone", "-q", "-b", base, f.target, work)
	run(f.t, work, "config", "user.email", "test@example.com")
	run(f.t, work, "config", "user.name", "test")
	run(f.t, work, "merge", "-q", "--squash", "origin/release/"+version)
	run(f.t, work, "commit", "-q", "-m", "build(release): "+version+" (#42)")
	run(f.t, work, "push", "-q", "origin", "HEAD:"+base)
}

// releaseBranch creates release-0.28 on upstream from v0.28.0, with the release tooling of
// main backported, as docs/dev/releasing.md says, and one commit of subject, as a backport adds
// it.
func (f *releaseFixture) releaseBranch(subject string) {
	f.t.Helper()
	f.bareReleaseBranch()
	run(f.t, f.dir, "switch", "-q", "-c", "backport", "upstream/release-0.28")
	writeFiles(f.t, f.dir, map[string]string{
		releasesDir + "/v0.28.0.json": "{}\n",
		".github/workflows/tests.yml": "branches: [main, 'release-*']\n",
	})
	run(f.t, f.dir, "add", "-A")
	run(f.t, f.dir, "commit", "-q", "-m", "ci: backport the release tooling")
	writeFiles(f.t, f.dir, map[string]string{"fix.txt": subject + "\n"})
	run(f.t, f.dir, "add", "fix.txt")
	run(f.t, f.dir, "commit", "-q", "-m", subject)
	run(f.t, f.dir, "push", "-q", "upstream", "HEAD:refs/heads/release-0.28")
	run(f.t, f.dir, "switch", "-q", "main")
	run(f.t, f.dir, "branch", "-q", "-D", "backport")
}

// bareReleaseBranch creates release-0.28 on upstream at v0.28.0, nothing backported.
func (f *releaseFixture) bareReleaseBranch() {
	f.t.Helper()
	run(f.t, f.dir, "push", "-q", "upstream", "v0.28.0^{commit}:refs/heads/release-0.28")
	run(f.t, f.dir, "fetch", "-q", "upstream")
}

func (f *releaseFixture) upstreamTags() []string { return f.tagsOf(f.upstream) }

func (f *releaseFixture) tagsOf(repo string) []string {
	f.t.Helper()
	out, err := git(repo, "tag", "--list")
	require.NoError(f.t, err)
	return strings.Fields(out)
}

// branches returns the branches of bare repository repo.
func (f *releaseFixture) branches(repo string) []string {
	f.t.Helper()
	out, err := git(repo, "branch", "--format=%(refname:short)")
	require.NoError(f.t, err)
	return strings.Fields(out)
}

func (f *releaseFixture) logged() string {
	b, _ := os.ReadFile(f.log)
	return string(b)
}

const fixturesDir = "platform/fabric/services/db/driver/sql/testdata"

// targets returns the target_commitish of every generate-notes call in log, in order.
func targets(log string) []string {
	var got []string
	for field := range strings.FieldsSeq(log) {
		if v, ok := strings.CutPrefix(field, "target_commitish="); ok {
			got = append(got, v)
		}
	}
	return got
}

func TestReleaseScript(t *testing.T) {
	t.Parallel()

	t.Run("prepare, pr and publish release the next minor", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)

		out := f.mustRelease("prepare")
		require.Contains(t, out, "make release-pr")
		staged, err := git(f.dir, "diff", "--cached", "--name-only")
		require.NoError(t, err)
		require.ElementsMatch(t, []string{
			releasesDir + "/v0.29.0.json", releasesDir + "/v0.29.0.md", fixturesDir + "/v0.29.0/SHA256SUMS",
		}, strings.Fields(staged))
		branch, err := git(f.dir, "branch", "--show-current")
		require.NoError(t, err)
		require.Equal(t, "release/v0.29.0", branch)

		f.mustRelease("pr")
		require.Contains(t, f.logged(), "gh pr create")
		msg, err := git(f.upstream, "log", "-1", "--format=%s%n%b", "release/v0.29.0")
		require.NoError(t, err)
		require.Contains(t, msg, "build(release): v0.29.0")
		require.Contains(t, msg, "Signed-off-by: test <test@example.com>")

		f.merge()
		out = f.mustRelease("publish")
		require.Contains(t, out, `build(release): v0.29.0 (#42)`)
		require.ElementsMatch(t, []string{"v0.28.0", "v0.29.0", "sub/v0.29.0"}, f.upstreamTags())
		require.Contains(t, f.logged(), "gh release create v0.29.0")

		// prepare and publish ask GitHub for the changelog of the same base: the commit
		// the release PR is based on, which is the parent of the squash commit.
		base, err := git(f.target, "rev-parse", "main^")
		require.NoError(t, err)
		require.Equal(t, []string{base, base}, targets(f.logged()))
	})

	t.Run("prepare, pr and publish release a patch from its release branch", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.releaseBranch("fix(view): a fix (backport #5)")

		// A patch has no fixtures, so it needs no Docker, and no upgrade guide.
		f.mustRelease("prepare", "VERSION=v0.28.1", "FAKE_NO_DOCKER=1")
		staged, err := git(f.dir, "diff", "--cached", "--name-only")
		require.NoError(t, err)
		require.ElementsMatch(t, []string{releasesDir + "/v0.28.1.json", releasesDir + "/v0.28.1.md"}, strings.Fields(staged))
		parent, err := git(f.dir, "log", "-1", "--format=%s", "HEAD")
		require.NoError(t, err)
		require.Equal(t, "fix(view): a fix (backport #5)", parent, "the release starts from the release branch")
		require.NotContains(t, f.logged(), "make pull-images-database")

		f.mustRelease("pr")
		require.Contains(t, f.logged(), "gh pr create -R hyperledger-labs/fabric-smart-client --base release-0.28 --head release/v0.28.1")

		f.mergeTo("v0.28.1", "release-0.28")
		run(t, f.dir, "switch", "-q", "main")
		out := f.mustRelease("publish")
		require.Contains(t, out, `build(release): v0.28.1 (#42)`)
		require.ElementsMatch(t, []string{"v0.28.0", "v0.28.1", "sub/v0.28.1"}, f.upstreamTags())
		tagged, err := git(f.upstream, "rev-parse", "v0.28.1^{commit}")
		require.NoError(t, err)
		head, err := git(f.upstream, "rev-parse", "release-0.28")
		require.NoError(t, err)
		require.Equal(t, head, tagged)
		require.Contains(t, f.logged(), "previous_tag_name=v0.28.0")
		require.Contains(t, f.logged(), "gh release create v0.28.1")
	})

	t.Run("publish asks which release when two are untagged", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.mustRelease("prepare")
		f.mustRelease("pr")
		f.merge()
		run(t, f.dir, "switch", "-q", "main")
		f.releaseBranch("fix: a fix (backport #5)")
		f.mustRelease("prepare", "VERSION=v0.28.1")
		f.mustRelease("pr")
		f.mergeTo("v0.28.1", "release-0.28")
		run(t, f.dir, "switch", "-q", "main")

		out, err := f.release("publish")
		require.Error(t, err)
		require.Contains(t, out, "more than one untagged release: v0.28.1 v0.29.0; set VERSION")
		f.mustRelease("publish", "VERSION=v0.28.1")
		require.ElementsMatch(t, []string{"v0.28.0", "v0.28.1", "sub/v0.28.1"}, f.upstreamTags())
	})

	t.Run("pr refuses a staged file that is not part of the release", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.mustRelease("prepare")
		writeFiles(t, f.dir, map[string]string{"go.mod": "module y\n"})
		run(t, f.dir, "add", "go.mod")
		out, err := f.release("pr")
		require.Error(t, err)
		require.Contains(t, out, "not part of the release: go.mod")
	})

	t.Run("pr updates an open release PR instead of opening another", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.mustRelease("prepare")
		out := f.mustRelease("pr", "FAKE_OPEN_PR=https://example.com/pull/7")
		require.Contains(t, out, "Updated the open release PR https://example.com/pull/7")
		require.NotContains(t, f.logged(), "gh pr create")
	})

	t.Run("publish leaves no tag behind when the push fails", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.mustRelease("prepare")
		f.mustRelease("pr")
		f.merge()
		// The remote refuses every tag.
		hook := filepath.Join(f.upstream, "hooks", "pre-receive")
		require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o700)) //nolint:gosec // an executable hook
		out, err := f.release("publish")
		require.Error(t, err)
		require.Contains(t, out, "no tag was created")
		local, err := git(f.dir, "tag", "--list", "*v0.29.0")
		require.NoError(t, err)
		require.Empty(t, local)

		require.NoError(t, os.Remove(hook))
		run(t, f.dir, "switch", "-q", "main")
		f.mustRelease("publish")
		require.ElementsMatch(t, []string{"v0.28.0", "v0.29.0", "sub/v0.29.0"}, f.upstreamTags())
	})

	t.Run("prepare refuses a release without an upgrade guide", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		out, err := f.release("prepare", "VERSION=v0.30.0")
		require.Error(t, err)
		require.Contains(t, out, "no upgrade guide")
	})

	t.Run("prepare refuses a remote that is not the main repository", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		out, err := f.release("prepare", "RELEASE_REPO_URL=github\\.com/hyperledger-labs/fabric-smart-client$")
		require.Error(t, err)
		require.Contains(t, out, "not hyperledger-labs/fabric-smart-client")
	})

	t.Run("a no at the prompt changes nothing", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		cmd := exec.Command(filepath.Join(f.dir, "scripts", "release.sh"), "prepare") //nolint:gosec // a test script
		cmd.Dir = f.dir
		cmd.Env = append(append([]string{}, f.env...), "YES=")
		cmd.Stdin = strings.NewReader("n\n")
		out, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(out), "stopped")
		require.Contains(t, string(out), "the upgrade guide of v0.29.0 is docs/upgrade/v0.28-to-v0.29.md")
		branch, err := git(f.dir, "branch", "--show-current")
		require.NoError(t, err)
		require.Equal(t, "main", branch)
	})

	t.Run("publish refuses stale notes and failed tests, then tags nothing", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.mustRelease("prepare")
		f.mustRelease("pr")
		f.merge()

		out, err := f.release("publish", "FAKE_NOTES=2")
		require.Error(t, err)
		require.Contains(t, out, "prepare the release again")

		run(t, f.dir, "switch", "-q", "main")
		out, err = f.release("publish", "FAKE_TESTS=failure")
		require.Error(t, err)
		require.Contains(t, out, "Tests did not pass")
		require.Equal(t, []string{"v0.28.0"}, f.upstreamTags())
	})

	t.Run("a fork releases on its own and links to itself", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.asFork()

		out := f.mustRelease("prepare", "VERSION=v0.29.0")
		require.Contains(t, out, "releasing in the fork someone/fabric-smart-client")
		f.mustRelease("pr")
		require.Contains(t, f.branches(f.target), "release/v0.29.0")
		require.NotContains(t, f.branches(f.upstream), "release/v0.29.0")
		f.merge()
		f.mustRelease("publish")

		require.ElementsMatch(t, []string{"v0.29.0", "sub/v0.29.0"}, f.tagsOf(f.target))
		require.Equal(t, []string{"v0.28.0"}, f.upstreamTags())
		// The fork's tags are not left in the checkout, where a release of the main
		// repository would see them.
		local, err := git(f.dir, "tag", "--list", "*v0.29.0")
		require.NoError(t, err)
		require.Empty(t, local)
		for line := range strings.SplitSeq(strings.TrimSpace(f.logged()), "\n") {
			switch {
			case strings.Contains(line, "generate-notes"):
				// The changelog comes from the main repository, which has the release tags.
				require.Contains(t, line, "repos/hyperledger-labs/fabric-smart-client/releases/generate-notes", line)
			case strings.HasPrefix(line, "gh ") && !strings.HasPrefix(line, "gh auth"):
				require.Contains(t, line, "someone/fabric-smart-client", line)
			}
			if strings.Contains(line, "scripts/release notes") {
				require.Contains(t, line, "--repo someone/fabric-smart-client")
			}
		}
		// The previous release comes from the main repository's tags, and the changelog is
		// generated in the main repository up to the commit the fork's release is based on,
		// which the main repository has.
		require.Contains(t, f.logged(), "previous_tag_name=v0.28.0")
		upstreamMain, err := git(f.upstream, "rev-parse", "main")
		require.NoError(t, err)
		for _, target := range targets(f.logged()) {
			require.Equal(t, upstreamMain, target)
		}
	})

	t.Run("the main repository is released only through its own remote", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.asFork()
		// RELEASE_REPO is the main repository, but REMOTE points to the fork.
		out, err := f.release("prepare", "RELEASE_REPO=hyperledger-labs/fabric-smart-client", "REMOTE=origin",
			"RELEASE_REPO_URL=github\\.com/hyperledger-labs/fabric-smart-client$")
		require.Error(t, err)
		require.Contains(t, out, "not hyperledger-labs/fabric-smart-client")
	})

	t.Run("a dry run writes nothing to GitHub", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.mustRelease("prepare", "DRY_RUN=1")
		out := f.mustRelease("pr", "DRY_RUN=1")
		require.Contains(t, out, "dry run, would run: git push")
		require.Contains(t, out, "dry run, would run: gh pr create")
		out = f.mustRelease("publish", "DRY_RUN=1")
		require.Contains(t, out, "checking release/v0.29.0 as if it were merged")
		// A dry release-pr committed nothing, so there is no release commit to list tags of.
		require.Contains(t, out, "tagging needs the release commit")
		require.Contains(t, out, "dry run, would run: ./scripts/tag-release.sh --push=upstream v0.29.0")
		require.Contains(t, out, "dry run, would run: gh release create v0.29.0")

		// Nothing was committed, pushed or tagged, and gh only read.
		subject, err := git(f.dir, "log", "-1", "--format=%s")
		require.NoError(t, err)
		require.Equal(t, "executables", subject)
		require.Equal(t, []string{"main"}, f.branches(f.upstream))
		require.Equal(t, []string{"v0.28.0"}, f.upstreamTags())
		for _, w := range []string{"gh pr create", "gh release create"} {
			require.NotContains(t, f.logged(), w)
		}
	})

	t.Run("the make targets pass the settings on", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		writeFiles(t, f.dir, map[string]string{"Makefile": makeTargets(t)})
		run(t, f.dir, "add", "Makefile")
		run(t, f.dir, "commit", "-q", "-m", "make")
		run(t, f.dir, "push", "-q", "upstream", "HEAD:main")
		// The make fake on the fixture's PATH would swallow the call; run the real make.
		mk, err := exec.LookPath("make")
		require.NoError(t, err)
		cmd := exec.Command(mk, "release-prepare", "DRY_RUN=1", "VERSION=v0.29.0") //nolint:gosec // a test
		cmd.Dir = f.dir
		cmd.Env = f.env
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		require.Contains(t, string(out), "dry run: nothing is written to GitHub")
		require.NotContains(t, f.logged(), "scripts/release bump", "VERSION reached the script")
	})

	t.Run("a release opened again after the merge is published from its second commit", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.mustRelease("prepare")
		f.mustRelease("pr")
		f.merge()
		first, err := git(f.upstream, "rev-parse", "main")
		require.NoError(t, err)

		// The release is opened again: a second release PR updates the files on main.
		run(t, f.dir, "switch", "-q", "main")
		f.mustRelease("prepare", "FAKE_NOTES=2")
		f.mustRelease("pr")
		work := filepath.Join(t.TempDir(), "merge2")
		run(t, f.dir, "clone", "-q", f.upstream, work)
		run(t, work, "config", "user.email", "test@example.com")
		run(t, work, "config", "user.name", "test")
		writeFiles(t, work, map[string]string{releasesDir + "/v0.29.0.md": "notes 2\n"})
		run(t, work, "commit", "-q", "-am", "build(release): v0.29.0 (#43)")
		run(t, work, "push", "-q", "origin", "HEAD:main")
		second, err := git(f.upstream, "rev-parse", "main")
		require.NoError(t, err)
		require.NotEqual(t, first, second)

		run(t, f.dir, "switch", "-q", "main")
		out := f.mustRelease("publish", "FAKE_NOTES=2")
		require.Contains(t, out, `build(release): v0.29.0 (#43)`)
		tagged, err := git(f.upstream, "rev-list", "-n1", "v0.29.0")
		require.NoError(t, err)
		require.Equal(t, second, tagged)
	})

	t.Run("a tag that was never pushed does not count as released", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.mustRelease("prepare")
		f.mustRelease("pr")
		f.merge()
		run(t, f.dir, "switch", "-q", "main")
		run(t, f.dir, "fetch", "-q", "upstream", "main")
		run(t, f.dir, "tag", "v0.29.0", "upstream/main") // local only, never pushed
		out, err := f.release("publish")
		// The script sees the release as untagged and goes on, until tag-release.sh refuses
		// the local tag that exists: the maintainer removes it, nothing reached the remote.
		require.Error(t, err)
		require.Contains(t, out, "release v0.29.0 is")
		require.Contains(t, out, "already exist")
		require.Equal(t, []string{"v0.28.0"}, f.upstreamTags())
	})

	t.Run("publish creates a missing GitHub release after the tags were pushed", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t)
		f.mustRelease("prepare")
		f.mustRelease("pr")
		f.merge()
		// A publish that pushed the tags and stopped before the GitHub release.
		run(t, f.dir, "fetch", "-q", "upstream", "main")
		run(t, f.dir, "push", "-q", "upstream", "upstream/main:refs/tags/v0.29.0")

		out := f.mustRelease("publish")
		require.Contains(t, out, "has no GitHub release")
		require.Contains(t, f.logged(), "gh release create v0.29.0")
	})
}

// TestReleaseScriptRefuses triggers every check of scripts/release.sh that stops a step, and
// checks that the step said why and left the remote as it was.
func TestReleaseScriptRefuses(t *testing.T) {
	t.Parallel()
	// Each case sets up a fixture, then runs step with env and expects the error want.
	merged := func(f *releaseFixture) {
		f.mustRelease("prepare")
		f.mustRelease("pr")
		f.merge()
	}
	for _, tc := range []struct {
		name  string
		setup func(f *releaseFixture)
		step  string
		env   []string
		want  string
	}{
		{name: "prepare without the remote", step: "prepare", env: []string{"REMOTE=nowhere"}, want: "no remote nowhere"},
		{name: "prepare on a dirty tree", setup: func(f *releaseFixture) {
			writeFiles(f.t, f.dir, map[string]string{"go.mod": "module y\n"})
		}, step: "prepare", want: "the working tree is not clean"},
		{name: "prepare without gh", step: "prepare", env: []string{"FAKE_NO_GH=1"}, want: "gh is not logged in"},
		{name: "prepare without Docker", step: "prepare", env: []string{"FAKE_NO_DOCKER=1"}, want: "Docker is not running"},
		{name: "prepare an invalid version", step: "prepare", env: []string{"VERSION=v0.29"}, want: "a release is vX.Y.Z"},
		{name: "prepare a patch without its release branch", step: "prepare", env: []string{"VERSION=v0.28.1"}, want: "upstream has no branch release-0.28"},
		{name: "prepare a patch with a feature", setup: func(f *releaseFixture) {
			f.releaseBranch("feat(view): a feature (backport #5)")
		}, step: "prepare", env: []string{"VERSION=v0.28.1"}, want: "a patch carries fixes only"},
		{name: "prepare a patch with a breaking change", setup: func(f *releaseFixture) {
			f.releaseBranch("fix(view)!: a breaking fix (backport #5)")
		}, step: "prepare", env: []string{"VERSION=v0.28.1"}, want: "a patch carries fixes only"},
		{name: "prepare a patch with a breaking change footer", setup: func(f *releaseFixture) {
			f.releaseBranch("fix(view): a fix (backport #5)\n\nBREAKING CHANGE: it breaks")
		}, step: "prepare", env: []string{"VERSION=v0.28.1"}, want: "fix(view): a fix (backport #5)"},
		{name: "prepare a patch on a branch without the release tooling", setup: func(f *releaseFixture) {
			f.bareReleaseBranch()
		}, step: "prepare", env: []string{"VERSION=v0.28.1"}, want: "release-0.28 lacks the release tooling of main"},
		{name: "prepare while another release PR is open", step: "prepare", env: []string{"FAKE_OTHER_RELEASE_PR=release/v0.30.0"}, want: "release PR release/v0.30.0 is open"},
		{name: "prepare a tagged release", step: "prepare", env: []string{"VERSION=v0.28.0"}, want: "v0.28.0 is tagged already"},
		{name: "prepare with no release before", setup: func(f *releaseFixture) {
			writeFiles(f.t, f.dir, map[string]string{"docs/upgrade/v0.0-to-v0.1.md": "guide\n"})
			run(f.t, f.dir, "add", "-A")
			run(f.t, f.dir, "commit", "-q", "-m", "guide")
			run(f.t, f.dir, "push", "-q", "upstream", "HEAD:main")
		}, step: "prepare", env: []string{"VERSION=v0.1.0"}, want: "no release before v0.1.0"},
		{name: "pr off a release branch", step: "pr", want: "check out release/vX.Y.Z first"},
		{name: "pr with unstaged changes", setup: func(f *releaseFixture) {
			f.mustRelease("prepare")
			writeFiles(f.t, f.dir, map[string]string{"go.mod": "module y\n"})
		}, step: "pr", want: "changes that are not staged"},
		{name: "pr with nothing staged", setup: func(f *releaseFixture) {
			run(f.t, f.dir, "switch", "-q", "-c", "release/v0.29.0")
		}, step: "pr", want: "nothing staged"},
		{name: "publish on a dirty tree", setup: func(f *releaseFixture) {
			writeFiles(f.t, f.dir, map[string]string{"go.mod": "module y\n"})
		}, step: "publish", want: "the working tree is not clean"},
		{name: "publish with nothing merged", step: "publish", want: "no release of hyperledger-labs/fabric-smart-client on upstream"},
		{name: "publish in a fork that has the main repository's releases on main", setup: func(f *releaseFixture) {
			// The fork's main has the data file of v0.28.0, which is tagged only in the main
			// repository, and the main repository's tags are fetched locally.
			writeFiles(f.t, f.dir, map[string]string{releasesDir + "/v0.28.0.json": "{}\n", releasesDir + "/v0.28.0.md": "notes\n"})
			run(f.t, f.dir, "add", "-A")
			run(f.t, f.dir, "commit", "-q", "-m", "build(release): v0.28.0 (#1)")
			run(f.t, f.dir, "push", "-q", "upstream", "HEAD:main")
			f.asFork()
			run(f.t, f.dir, "push", "-q", "origin", "HEAD:main")
		}, step: "publish", env: []string{"FAKE_RELEASED="}, want: "no release of someone/fabric-smart-client on origin"},
		{name: "publish when every release is out", setup: func(f *releaseFixture) {
			merged(f)
			run(f.t, f.dir, "switch", "-q", "main")
			f.mustRelease("publish")
		}, step: "publish", env: []string{"FAKE_RELEASED=1"}, want: "no untagged release on upstream"},
		{name: "publish a data file a release PR did not add", setup: func(f *releaseFixture) {
			writeFiles(f.t, f.dir, map[string]string{releasesDir + "/v0.29.0.json": "{}\n"})
			run(f.t, f.dir, "add", "-A")
			run(f.t, f.dir, "commit", "-q", "-m", "docs: add a file")
			run(f.t, f.dir, "push", "-q", "upstream", "HEAD:main")
		}, step: "publish", want: "not a release PR"},
		{name: "publish with two untagged releases", setup: func(f *releaseFixture) {
			writeFiles(f.t, f.dir, map[string]string{releasesDir + "/v0.29.0.json": "{}\n", releasesDir + "/v0.30.0.json": "{}\n"})
			run(f.t, f.dir, "add", "-A")
			run(f.t, f.dir, "commit", "-q", "-m", "build(release): two")
			run(f.t, f.dir, "push", "-q", "upstream", "HEAD:main")
		}, step: "publish", want: "more than one untagged release"},
		{
			name: "publish a version that is not untagged", setup: merged, step: "publish", env: []string{"VERSION=v0.30.0"},
			want: "no release v0.30.0 on upstream",
		},
		{name: "publish before Tests ran on the release commit", setup: func(f *releaseFixture) {
			merged(f)
			run(f.t, f.dir, "switch", "-q", "main")
		}, step: "publish", env: []string{"FAKE_TESTS="}, want: "Tests did not pass on the release commit"},
		{name: "dry-run publish with nothing prepared", setup: func(f *releaseFixture) {
			run(f.t, f.dir, "switch", "-q", "-c", "release/v0.29.0")
		}, step: "publish", env: []string{"DRY_RUN=1"}, want: "nothing staged or committed"},
		{name: "dry-run publish with stale notes", setup: func(f *releaseFixture) {
			f.mustRelease("prepare")
		}, step: "publish", env: []string{"DRY_RUN=1", "FAKE_NOTES=2"}, want: "prepare the release again"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newReleaseFixture(t)
			if tc.setup != nil {
				tc.setup(f)
			}
			branches, tags, logged := f.branches(f.upstream), f.upstreamTags(), f.logged()
			out, err := f.release(tc.step, tc.env...)
			require.Error(t, err, out)
			require.Contains(t, out, tc.want)
			// The refused step wrote nothing: no tag or branch on the remote, no PR or release.
			require.Equal(t, tags, f.upstreamTags())
			require.Equal(t, branches, f.branches(f.upstream))
			added := strings.TrimPrefix(f.logged(), logged)
			require.NotContains(t, added, "gh pr create")
			require.NotContains(t, added, "gh release create")
		})
	}
}

// makeTargets returns the release targets of the repository's Makefile.
func makeTargets(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	require.NoError(t, err)
	var targets []string
	keep := false
	for line := range strings.Lines(string(b)) {
		if strings.HasPrefix(line, ".PHONY: release-prepare") {
			keep = true
		}
		if keep && strings.TrimSpace(line) == "" {
			break
		}
		if keep {
			targets = append(targets, line)
		}
	}
	require.NotEmpty(t, targets)
	return strings.Join(targets, "")
}
