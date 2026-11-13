/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"cmp"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// semver is a release version vMAJOR.MINOR.PATCH.
type semver struct{ Major, Minor, Patch int }

var versionRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

// parseVersion parses a release tag. Module tags (integration/v…) and pre-releases are not
// release tags.
func parseVersion(s string) (semver, bool) {
	var v semver
	if !versionRe.MatchString(s) {
		return v, false
	}
	_, err := fmt.Sscanf(s, "v%d.%d.%d", &v.Major, &v.Minor, &v.Patch)
	return v, err == nil
}

func (v semver) String() string { return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch) }

func (v semver) compare(o semver) int {
	return cmp.Or(cmp.Compare(v.Major, o.Major), cmp.Compare(v.Minor, o.Minor), cmp.Compare(v.Patch, o.Patch))
}

// gitRaw runs git in dir and returns its standard output.
func gitRaw(dir string, args ...string) ([]byte, error) {
	return command("", nil, "git", append([]string{"-C", dir}, args...)...)
}

// git runs git in dir and returns its trimmed standard output.
func git(dir string, args ...string) (string, error) {
	out, err := gitRaw(dir, args...)
	return strings.TrimSpace(string(out)), err
}

// showAt returns the file at path in the tree of tag, and false if the tree has no such file.
// A tag or tree git cannot read is an error.
func showAt(dir string, tag semver, path string) ([]byte, bool, error) {
	ref := tag.String() + "^{commit}"
	if found, err := git(dir, "ls-tree", "--name-only", ref, "--", path); err != nil || found == "" {
		return nil, false, err
	}
	out, err := gitRaw(dir, "show", tag.String()+":"+path)
	return out, err == nil, err
}

// releaseTags returns the release tags of the repository in dir, lowest first.
func releaseTags(dir string) ([]semver, error) {
	out, err := git(dir, "tag", "--list", "v*")
	if err != nil {
		return nil, err
	}
	var tags []semver
	for line := range strings.Lines(out) {
		if v, ok := parseVersion(strings.TrimSpace(line)); ok {
			tags = append(tags, v)
		}
	}
	slices.SortFunc(tags, semver.compare)
	return tags, nil
}

// nextMinor returns the minor release after the highest of tags.
func nextMinor(tags []semver) (semver, error) {
	if len(tags) == 0 {
		return semver{}, errors.New("no release tag; fetch the tags first")
	}
	last := slices.MaxFunc(tags, semver.compare)
	return semver{Major: last.Major, Minor: last.Minor + 1}, nil
}

// previousRelease returns the release v follows, which its changelog starts from. A patch
// vX.Y.Z follows vX.Y.(Z-1). A minor follows the previous minor vX.(Y-1).0, not a patch of it,
// which is on a release branch; the first minor of a major follows the highest minor below it.
// The release v follows must be in tags, so that no version is skipped.
func previousRelease(tags []semver, v semver) (semver, error) {
	var want semver
	switch {
	case v.Patch > 0:
		want = semver{v.Major, v.Minor, v.Patch - 1}
	case v.Minor > 0:
		want = semver{v.Major, v.Minor - 1, 0}
	default:
		var prev *semver
		for _, t := range tags {
			if t.Patch == 0 && t.compare(v) < 0 && (prev == nil || t.compare(*prev) > 0) {
				prev = &t
			}
		}
		if prev == nil {
			return semver{}, errors.Errorf("no release before %s; fetch the tags first", v)
		}
		return *prev, nil
	}
	if !slices.Contains(tags, want) {
		if !slices.ContainsFunc(tags, func(t semver) bool { return t.compare(v) < 0 }) {
			return semver{}, errors.Errorf("no release before %s; fetch the tags first", v)
		}
		return semver{}, errors.Errorf("no release %s before %s; fetch the tags first, or release %s", want, v, want)
	}
	return want, nil
}

// modules returns the tags that release v of the modules committed at HEAD in dir: the root
// module as v, every other module except tools as <dir>/v, sorted. These are the tags
// scripts/tag-release.sh creates, from the same tree.
func modules(dir string, v semver) ([]string, error) {
	dirs, err := moduleDirs(dir, "HEAD")
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, d := range dirs {
		if d == "." {
			tags = append(tags, v.String())
		} else {
			tags = append(tags, d+"/"+v.String())
		}
	}
	slices.Sort(tags)
	return tags, nil
}

// moduleDirs returns the directories of the released modules in the tree of rev in dir: every
// directory with a go.mod except tools, whose module is never released. The root module is ".".
func moduleDirs(dir, rev string) ([]string, error) {
	out, err := git(dir, "ls-tree", "-r", "--name-only", rev)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for line := range strings.Lines(out) {
		file := strings.TrimSpace(line)
		if path.Base(file) == "go.mod" && path.Dir(file) != "tools" {
			dirs = append(dirs, path.Dir(file))
		}
	}
	return dirs, nil
}
