/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// apiPasses are the builds whose Go API a patch keeps: the default build, and the build with
// tag pkcs11, which has other files in some packages. Each pass sets its build tags after the
// caller's GOFLAGS, so the caller's other flags apply and its tags do not. The passes build for
// the host OS: no FSC file is OS- or arch-specific, and building for another OS turns cgo off,
// without which the pkcs11 build of integration does not load.
var apiPasses = []struct{ name, tags string }{
	{"default", ""},
	{"pkcs11", "pkcs11"},
}

// generatedRe matches the first line of a generated Go file, as `go generate` defines it.
var generatedRe = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// apiDiff fails when the Go API of the released modules at rev head in dir is incompatible
// with their API at release base: exactly what `apidiff -m -incompatible` reports, in either
// pass of apiPasses, without the generated test doubles (see testDouble). Both trees come
// from git, so files a checkout does not track take no part. The error lists each change,
// grouped by module, with the passes that found it.
func apiDiff(dir string, base semver, head string) error {
	tmp, err := os.MkdirTemp("", "apidiff")
	if err != nil {
		return errors.Wrap(err, "failed creating a temporary directory")
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	tool, err := buildAPIDiff(tmp)
	if err != nil {
		return err
	}
	baseTree, headTree := filepath.Join(tmp, "base"), filepath.Join(tmp, "head")
	if err := archive(dir, base.String(), baseTree); err != nil {
		return err
	}
	if err := archive(dir, head, headTree); err != nil {
		return err
	}
	mods, err := moduleDirs(dir, base.String())
	if err != nil {
		return err
	}

	var report strings.Builder
	n := 0
	for _, m := range mods {
		modPath, err := modulePath(filepath.Join(baseTree, m))
		if err != nil {
			return err
		}
		// The changes in the order apidiff reports them, and the passes that found each.
		var changes []string
		passes := map[string][]string{}
		for _, p := range apiPasses {
			env := append(os.Environ(), "GOFLAGS="+strings.TrimSpace(os.Getenv("GOFLAGS")+" -tags="+p.tags))
			files := [2]string{}
			for i, tree := range []string{baseTree, headTree} {
				files[i] = filepath.Join(tmp, fmt.Sprintf("%s-%d-%s", p.name, i, strings.ReplaceAll(m, "/", "_")))
				if _, err := command(filepath.Join(tree, m), env, tool, "-m", "-w", files[i], modPath); err != nil {
					return errors.Wrapf(err, "failed exporting the API of %s (%s)", modPath, p.name)
				}
			}
			out, err := command(tmp, env, tool, "-m", "-incompatible", files[0], files[1])
			if err != nil {
				return errors.Wrapf(err, "failed comparing the API of %s (%s)", modPath, p.name)
			}
			for line := range strings.Lines(string(out)) {
				c, ok := strings.CutPrefix(strings.TrimSpace(line), "- ")
				if !ok {
					continue // the heading
				}
				pkg := filepath.Join(m, filepath.FromSlash(strings.TrimPrefix(changedPackage(c, modPath), modPath)))
				if testDouble(filepath.Join(headTree, pkg)) || testDouble(filepath.Join(baseTree, pkg)) {
					continue
				}
				if passes[c] == nil {
					changes = append(changes, c)
				}
				passes[c] = append(passes[c], p.name)
			}
		}
		if len(changes) == 0 {
			continue
		}
		fmt.Fprintf(&report, "\n%s:\n", modPath)
		for _, c := range changes {
			fmt.Fprintf(&report, "- %s (%s)\n", c, strings.Join(passes[c], ", "))
		}
		n += len(changes)
	}
	if n > 0 {
		return errors.Errorf("%s has %d incompatible API changes since %s; a patch keeps the API "+
			"(docs/dev/releasing.md, The Go API of a patch):\n%s", head, n, base, report.String())
	}
	return nil
}

// buildAPIDiff builds apidiff into dir at the version tools/go.mod pins, and returns its path.
// tools is next to the go.mod of the module the release command runs in.
func buildAPIDiff(dir string) (string, error) {
	gomod, err := command("", nil, "go", "env", "GOMOD")
	if err != nil {
		return "", err
	}
	tools := filepath.Join(filepath.Dir(strings.TrimSpace(string(gomod))), "tools")
	tool := filepath.Join(dir, "apidiff")
	if _, err := command(tools, nil, "go", "build", "-o", tool, "golang.org/x/exp/cmd/apidiff"); err != nil {
		return "", errors.Wrap(err, "failed building apidiff")
	}
	return tool, nil
}

// archive writes the tree of rev in dir to the new directory to.
func archive(dir, rev, to string) error {
	tar, err := gitRaw(dir, "archive", "--format=tar", rev+"^{tree}")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(to, 0o750); err != nil {
		return errors.Wrapf(err, "failed creating %s", to)
	}
	cmd := exec.Command("tar", "-x", "-C", to) //nolint:gosec // a temporary directory
	cmd.Stdin = bytes.NewReader(tar)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "failed extracting %s: %s", rev, out)
	}
	return nil
}

// modulePath returns the module path of the go.mod in dir.
func modulePath(dir string) (string, error) {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", errors.Wrap(err, "failed reading go.mod")
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if p, ok := strings.CutPrefix(strings.TrimSpace(s.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(p), `"`), nil
		}
	}
	return "", errors.Errorf("%s/go.mod has no module line", dir)
}

// changedPackage returns the import path of the package a change apidiff reports for module
// modPath is in. apidiff writes a removed package as "package <import path>: removed", a
// change in package a/b of the module as "./a/b.<object>: <change>", and a change in the
// module's root package as "<object>: <change>".
func changedPackage(change, modPath string) string {
	subject, _, _ := strings.Cut(change, ": ")
	if p, ok := strings.CutPrefix(subject, "package "); ok {
		return p
	}
	rel, ok := strings.CutPrefix(subject, "./")
	if !ok {
		return modPath
	}
	slash := strings.LastIndex(rel, "/")
	if dot := strings.Index(rel[slash+1:], "."); dot >= 0 {
		rel = rel[:slash+1+dot]
	}
	return modPath + "/" + rel
}

// testDouble tells whether dir holds a generated test double: its name is mock, mocks, fake
// or fakes, and each of its Go files is generated. A hand-written double is API like any other
// code, and so is generated code elsewhere, such as the protobuf files.
func testDouble(dir string) bool {
	switch path.Base(filepath.ToSlash(dir)) {
	case "mock", "mocks", "fake", "fakes":
	default:
		return false
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	files = slices.DeleteFunc(files, func(f string) bool { return strings.HasSuffix(f, "_test.go") })
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return false
		}
		first, _, _ := strings.Cut(string(raw), "\n")
		if !generatedRe.MatchString(strings.TrimSpace(first)) {
			return false
		}
	}
	return len(files) > 0
}

// command runs name with args in dir ("" for the current directory) with environment env (nil
// for this process's), and returns its standard output.
func command(dir string, env []string, name string, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	// The commands are git, go, tar and the apidiff built here, with fixed subcommands and
	// repository paths, never shell input.
	cmd := exec.Command(name, args...) //nolint:gosec // see above
	cmd.Dir, cmd.Env, cmd.Stderr = dir, env, &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrapf(err, "%s %s: %s", path.Base(name), strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
