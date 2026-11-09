/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	nwonode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// fscModule is the root module of FSC. The integration module and the two
// submodules it requires are paths below it.
const fscModule = "github.com/hyperledger-labs/fabric-smart-client"

// releaseModules are the FSC modules a node's generated main depends on, as
// subdirectories of fscModule; "" is the root module. A release build pins
// each of them, because the integration module requires the other three at
// the zero pseudo-version and fills them in with replace directives, which Go
// ignores outside the main module.
var releaseModules = []string{
	"",
	"integration",
	"platform/fabric/services/state/cc/query",
	"platform/view/services/comm/host/libp2p",
}

// buildAtRelease builds the generated main of node against the FSC release
// release and returns the path of the binary. release is a module version, a
// tag such as v0.28.0 or a commit, or the absolute path of a checkout of the
// repository. The main package is the one nwo generated for this test, so every
// package it imports, the views and SDKs the topology registers among them,
// must exist at release.
//
// The replicas of a node share its generated main and its binary, in a directory
// next to the main, one per release. Every call rebuilds it, because nwo
// regenerates the main of each network; Go's build cache makes a rebuild of
// unchanged sources cheap. It is built without the race detector and coverage,
// outside any workspace and with GOFLAGS cleared in the environment; GOFLAGS set
// with go env -w still apply.
func (p *Platform) buildAtRelease(node *nwonode.Replica, release string) (string, error) {
	dir := filepath.Join(p.NodeCmdDir(node)+"-release", sanitizeRelease(release))
	bin := filepath.Join(dir, "node")

	src, err := os.ReadFile(p.NodeCmdPath(node))
	if err != nil {
		return "", err
	}
	targets, err := releaseTargets(release)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), src, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(releaseGoMod(targets)), 0o644); err != nil {
		return "", err
	}
	logger.Infof("building [%s] at FSC release [%s]", node.Name, release)
	if _, err := releaseGo(dir, "build", "-mod=mod", "-o", bin, "."); err != nil {
		return "", errors.Wrapf(err, "failed to build [%s] at FSC release [%s]", node.Name, release)
	}
	return bin, nil
}

// releaseTargets returns, for each of releaseModules, what the release build
// replaces it with: a directory of the checkout release, or the module version
// release resolves to. A tag of the root module names the release of every
// module: v0.28.0 resolves the integration module to its tag integration/v0.28.0.
func releaseTargets(release string) (map[string]string, error) {
	if err := checkRelease(release); err != nil {
		return nil, err
	}
	targets := map[string]string{}
	if filepath.IsAbs(release) {
		for _, m := range releaseModules {
			targets[m] = filepath.Join(release, m)
		}
		return targets, nil
	}
	for _, m := range releaseModules {
		out, err := releaseGo(os.TempDir(), "list", "-m", "-f", "{{.Version}}", modulePath(m)+"@"+release)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to resolve [%s] at [%s]", modulePath(m), release)
		}
		targets[m] = strings.TrimSpace(out)
	}
	return targets, nil
}

// releaseGoMod returns the go.mod of a release build. targets maps each of
// releaseModules to a module version or a directory, as releaseTargets returns.
// It names no go version: the build fills in the toolchain's own.
func releaseGoMod(targets map[string]string) string {
	var b strings.Builder
	b.WriteString("module fscrelease\n\nrequire (\n")
	for _, m := range releaseModules[:2] {
		// A directory is replaced anyway, so its require only needs a valid version.
		v := targets[m]
		if filepath.IsAbs(v) {
			v = "v0.0.0-00010101000000-000000000000"
		}
		b.WriteString("\t" + modulePath(m) + " " + v + "\n")
	}
	b.WriteString(")\n\nreplace (\n")
	for _, m := range releaseModules {
		b.WriteString("\t" + modulePath(m) + " => ")
		if filepath.IsAbs(targets[m]) {
			b.WriteString(targets[m] + "\n")
		} else {
			b.WriteString(modulePath(m) + " " + targets[m] + "\n")
		}
	}
	b.WriteString(")\n")
	return b.String()
}

// ReleaseHasPackage reports whether the FSC release release, as buildAtRelease
// takes it, contains the package pkg of the integration module, given as a path
// below the module such as fabric/mixedversion/views. A suite whose views are
// missing at a release cannot run a node of that release.
func ReleaseHasPackage(release, pkg string) (bool, error) {
	if err := checkRelease(release); err != nil {
		return false, err
	}
	dir := filepath.Join(release, "integration")
	if !filepath.IsAbs(release) {
		out, err := releaseGo(os.TempDir(), "mod", "download", "-json", modulePath("integration")+"@"+release)
		if err != nil {
			return false, errors.Wrapf(err, "failed to download [%s] at [%s]", modulePath("integration"), release)
		}
		var mod struct{ Dir string }
		if err := json.Unmarshal([]byte(out), &mod); err != nil {
			return false, errors.Wrap(err, "failed to parse go mod download output")
		}
		dir = mod.Dir
	}
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(pkg)))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func modulePath(m string) string {
	if m == "" {
		return fscModule
	}
	return fscModule + "/" + m
}

// checkRelease refuses a relative path, which would be taken for a module version
// and resolve against whichever directory the test runs in.
func checkRelease(release string) error {
	if !filepath.IsAbs(release) && (strings.HasPrefix(release, ".") || strings.ContainsRune(release, filepath.Separator)) {
		return errors.Errorf("FSC release [%s]: a checkout must be given as an absolute path", release)
	}
	return nil
}

// sanitizeRelease turns release, a version or a path, into a directory name.
func sanitizeRelease(release string) string {
	return strings.ReplaceAll(release, string(filepath.Separator), "_")
}

// releaseGo runs the go command in dir for a release build: outside any
// workspace and with GOFLAGS cleared in the environment, which may name the race
// detector, coverage or -mod settings of the calling module. The rest of the go
// environment, such as GOPROXY, applies.
func releaseGo(dir string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", errors.Wrapf(err, "go %s: %s", strings.Join(args, " "), stderr.String())
	}
	return string(out), nil
}
