/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"cmp"
	"fmt"
	"os/exec"
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
	var stderr bytes.Buffer
	// The arguments are fixed subcommands, tags and repository paths, never shell input.
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec // see above
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrapf(err, "git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return out, nil
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

// previousRelease returns the highest of tags below v.
func previousRelease(tags []semver, v semver) (semver, error) {
	var prev *semver
	for _, t := range tags {
		if t.compare(v) < 0 && (prev == nil || t.compare(*prev) > 0) {
			prev = &t
		}
	}
	if prev == nil {
		return semver{}, errors.Errorf("no release before %s; fetch the tags first", v)
	}
	return *prev, nil
}

// modules returns the tags that release v of the modules committed at HEAD in dir: the root
// module as v, every other module except tools as <dir>/v, sorted. These are the tags
// scripts/tag-release.sh creates, from the same tree.
func modules(dir string, v semver) ([]string, error) {
	out, err := git(dir, "ls-tree", "-r", "--name-only", "HEAD")
	if err != nil {
		return nil, err
	}
	var tags []string
	for line := range strings.Lines(out) {
		file := strings.TrimSpace(line)
		if path.Base(file) != "go.mod" {
			continue
		}
		switch d := path.Dir(file); d {
		case "tools":
		case ".":
			tags = append(tags, v.String())
		default:
			tags = append(tags, d+"/"+v.String())
		}
	}
	slices.Sort(tags)
	return tags, nil
}
