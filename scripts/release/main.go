/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Command release writes the release data file, the release notes and the backport rules of
// the next FSC release, and checks that a patch keeps the Go API of the release before it.
// Run it from the repository root; docs/upgrade/releases/README.md shows how to preview a
// release.
//
//	release bump
//	release previous --version vX.Y.Z
//	release data --version vX.Y.Z --report report.json
//	release notes --data vX.Y.Z.json --changelog changelog.md [--repo owner/name]
//	release mergify
//	release check --report report.json
//	release apidiff --base vX.Y.Z [--head rev]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// releasesDir holds the release data files and notes, relative to the repository root.
const releasesDir = "docs/upgrade/releases"

const usage = `usage:
  release bump
  release previous --version vX.Y.Z
  release data --version vX.Y.Z --report report.json
  release notes --data vX.Y.Z.json --changelog changelog.md [--repo owner/name]
  release mergify
  release check --report report.json
  release apidiff --base vX.Y.Z [--head rev]
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	out, err := runCommand(".", os.Args[1], os.Args[2:])
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
	fmt.Print(out)
}

// runCommand runs the subcommand cmd with args in the repository dir and returns its output.
func runCommand(dir, cmd string, args []string) (string, error) {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	version := fs.String("version", "", "the version to release, vX.Y.Z")
	report := fs.String("report", "", "the output of `refnode version --json`")
	data := fs.String("data", "", "the release data file")
	changelog := fs.String("changelog", "", "the changelog GitHub generated for the release")
	repo := fs.String("repo", mainRepo, "the GitHub repository the notes link to, owner/name")
	base := fs.String("base", "", "the release whose Go API a patch keeps, vX.Y.Z")
	head := fs.String("head", "HEAD", "the revision whose Go API is checked")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	required := func(names ...string) error {
		for _, n := range names {
			if fs.Lookup(n).Value.String() == "" {
				return errors.Errorf("%s needs --%s", cmd, n)
			}
		}
		return nil
	}
	switch cmd {
	case "bump":
		tags, err := releaseTags(dir)
		if err != nil {
			return "", err
		}
		v, err := nextMinor(tags)
		if err != nil {
			return "", err
		}
		return v.String() + "\n", nil
	case "previous":
		if err := required("version"); err != nil {
			return "", err
		}
		v, ok := parseVersion(*version)
		if !ok {
			return "", errors.Errorf("invalid version [%s], want vX.Y.Z", *version)
		}
		tags, err := releaseTags(dir)
		if err != nil {
			return "", err
		}
		prev, err := previousRelease(tags, v)
		if err != nil {
			return "", err
		}
		return prev.String() + "\n", nil
	case "data":
		if err := required("version", "report"); err != nil {
			return "", err
		}
		v, ok := parseVersion(*version)
		if !ok {
			return "", errors.Errorf("invalid version [%s], want vX.Y.Z", *version)
		}
		r, err := readJSON[Report](*report)
		if err != nil {
			return "", err
		}
		d, err := newData(dir, v, r)
		if err != nil {
			return "", err
		}
		raw, err := d.render()
		return string(raw), err
	case "notes":
		if err := required("data", "changelog"); err != nil {
			return "", err
		}
		d, err := readJSON[Data](*data)
		if err != nil {
			return "", err
		}
		c, err := os.ReadFile(*changelog)
		if err != nil {
			return "", errors.Wrap(err, "failed reading the changelog")
		}
		return notes(dir, *repo, d, string(c))
	case "mergify":
		return mergify(dir)
	case "check":
		if err := required("report"); err != nil {
			return "", err
		}
		r, err := readJSON[Report](*report)
		if err != nil {
			return "", err
		}
		return "", errors.Join(check(dir, r), checkMergify(dir))
	case "apidiff":
		if err := required("base"); err != nil {
			return "", err
		}
		v, ok := parseVersion(*base)
		if !ok {
			return "", errors.Errorf("invalid version [%s], want vX.Y.Z", *base)
		}
		return "", apiDiff(dir, v, *head)
	default:
		return "", errors.Errorf("unknown command [%s]\n%s", cmd, usage)
	}
}

func readJSON[T any](file string) (T, error) {
	var v T
	raw, err := os.ReadFile(file)
	if err != nil {
		return v, errors.Wrapf(err, "failed reading %s", file)
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, errors.Wrapf(err, "failed parsing %s", file)
	}
	return v, nil
}

// check fails when a release data file in dir was edited. A file whose release is tagged must
// equal the file at the tag: a released record never changes. A tag without a file is a release
// cut before the tooling, and its file is not checked. A file without a tag (a release PR) must
// equal what newData generates from report r.
func check(dir string, r Report) error {
	files, err := filepath.Glob(filepath.Join(dir, filepath.FromSlash(releasesDir), "v*.json"))
	if err != nil {
		return errors.Wrap(err, "failed listing the release data files")
	}
	tags, err := releaseTags(dir)
	if err != nil {
		return err
	}
	tagged := map[semver]bool{}
	for _, t := range tags {
		tagged[t] = true
	}
	for _, f := range files {
		v, ok := parseVersion(strings.TrimSuffix(filepath.Base(f), ".json"))
		if !ok {
			continue
		}
		rel := path.Join(releasesDir, filepath.Base(f))
		got, err := os.ReadFile(f)
		if err != nil {
			return errors.Wrapf(err, "failed reading %s", rel)
		}
		if tagged[v] {
			released, ok, err := showAt(dir, v, rel)
			if err != nil {
				return err
			}
			if ok && !bytes.Equal(got, released) {
				return errors.Errorf("%s differs from %s: a released record never changes", rel, v)
			}
			continue
		}
		d, err := newData(dir, v, r)
		if err != nil {
			return err
		}
		want, err := d.render()
		if err != nil {
			return err
		}
		if !bytes.Equal(got, want) {
			return errors.Errorf(`%s is not what "data" generates: it was edited, or the tree changed `+
				`since it was generated (a schema step, a component range, a pin); regenerate it`, rel)
		}
	}
	return nil
}
