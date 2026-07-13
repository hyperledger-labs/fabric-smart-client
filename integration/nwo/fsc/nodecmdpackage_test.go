/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
)

// writeModule creates a temp module rooted at <tmp>/<rootDirName> declaring the given
// module path, with a nested package directory under it, and returns the nested
// directory's absolute path.
func writeModule(t *testing.T, rootDirName, modulePath string, nestedPkg ...string) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), rootDirName)
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module "+modulePath+"\n\ngo 1.21\n"),
		0o644,
	))

	nested := filepath.Join(append([]string{root}, nestedPkg...)...)
	require.NoError(t, os.MkdirAll(nested, 0o755))

	return nested
}

func TestNodeCmdPackage_ResolvesViaGoModRegardlessOfCheckoutDirName(t *testing.T) { //nolint:paralleltest
	// Simulate a git worktree checkout: the on-disk directory is named "panurus-1226"
	// (as git worktree creates by default) even though go.mod still declares the
	// canonical module path "github.com/LFDT-Panurus/panurus". Before the fix,
	// NodeCmdPackage derived the import path from the checkout directory name via
	// $GOPATH/src string slicing, which produced a bogus path containing
	// ".../panurus-1226/..." that no go.mod actually declares.
	gomega.RegisterTestingT(t)

	nested := writeModule(t, "panurus-1226", "github.com/LFDT-Panurus/panurus",
		"integration", "token", "fungible", "dlogx")

	t.Chdir(nested)

	var p *Platform
	replica := node.NewReplica(&node.Peer{Name: "lib-p2p-bootstrap-node"}, "bootstrap.0")

	got := p.NodeCmdPackage(replica)

	want := "github.com/LFDT-Panurus/panurus/integration/token/fungible/dlogx/out/cmd/lib-p2p-bootstrap-node"
	assert.Equal(t, want, got)
}

func TestNodeCmdPackage_MatchesCanonicalCheckoutDirName(t *testing.T) { //nolint:paralleltest
	// Sanity check: when the checkout directory name does match the module name
	// (the "normal", non-worktree case), the resolved import path is unchanged.
	gomega.RegisterTestingT(t)

	nested := writeModule(t, "panurus", "github.com/LFDT-Panurus/panurus",
		"integration", "token", "fungible", "dlogx")

	t.Chdir(nested)

	var p *Platform
	replica := node.NewReplica(&node.Peer{Name: "lib-p2p-bootstrap-node"}, "bootstrap.0")

	got := p.NodeCmdPackage(replica)

	want := "github.com/LFDT-Panurus/panurus/integration/token/fungible/dlogx/out/cmd/lib-p2p-bootstrap-node"
	assert.Equal(t, want, got)
}

func TestNodeCmdPackage_FallsBackWhenNoGoModResolvable(t *testing.T) { //nolint:paralleltest
	// When run from a directory outside of any go module (no go.mod anywhere in the
	// tree) and outside of $GOPATH/src, NodeCmdPackage falls back to a relative path.
	gomega.RegisterTestingT(t)

	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GOPATH", filepath.Join(dir, "unrelated-gopath"))

	var p *Platform
	replica := node.NewReplica(&node.Peer{Name: "lib-p2p-bootstrap-node"}, "bootstrap.0")

	got := p.NodeCmdPackage(replica)

	want := "./out/cmd/lib-p2p-bootstrap-node"
	assert.Equal(t, want, got)
}

func TestGoModuleInfo(t *testing.T) { //nolint:paralleltest
	nested := writeModule(t, "some-worktree-dir", "github.com/example/proj", "pkg", "sub")

	modPath, modDir, err := goModuleInfo(nested)
	require.NoError(t, err)

	assert.Equal(t, "github.com/example/proj", modPath)

	rootDir := filepath.Dir(filepath.Dir(nested)) // strip "pkg/sub"
	resolvedRoot, err := filepath.EvalSymlinks(rootDir)
	require.NoError(t, err)
	resolvedModDir, err := filepath.EvalSymlinks(modDir)
	require.NoError(t, err)
	assert.Equal(t, resolvedRoot, resolvedModDir)
}

func TestGoModuleInfo_ErrorsOutsideModule(t *testing.T) { //nolint:paralleltest
	dir := t.TempDir()

	_, _, err := goModuleInfo(dir)
	assert.Error(t, err)
}
