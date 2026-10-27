/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
)

const testPkg = "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/out/cmd/buildertest"

// writeMain writes a main package that prints msg under ./out/cmd, where nwo
// generates the main package of every FSC node.
func writeMain(t *testing.T, msg string) {
	t.Helper()
	dir := filepath.Join("out", "cmd", "buildertest")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	src := "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Print(\"" + msg + "\") }\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644))
}

func run(t *testing.T, bin string) string {
	t.Helper()
	out, err := exec.Command(bin).Output()
	require.NoError(t, err)
	return string(out)
}

func newServer(t *testing.T) *BuildServer {
	t.Helper()
	s := NewBuildServer()
	s.Serve()
	t.Cleanup(func() { s.Shutdown(false) })
	return s
}

// TestBuildServerSharesBinaries covers two networks of one process, one after
// the other, generating different main packages at the same path, as
// fabric/iou does for websocket and libp2p: each network runs the binary of
// its own sources.
func TestBuildServerSharesBinaries(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join("out", "cmd", "buildertest")) })
	t.Cleanup(CleanupBuildArtifacts)

	writeMain(t, "v1")
	first := newServer(t).Client().Build(testPkg)
	require.Equal(t, "v1", run(t, first))

	writeMain(t, "v2")
	second := newServer(t)
	bin := second.Client().Build(testPkg)
	require.Equal(t, first, bin, "servers with the same flags share the binary")
	require.Equal(t, "v2", run(t, bin), "a new server rebuilds changed sources")

	race := newServer(t)
	race.EnableRaceDetector()
	require.NotEqual(t, bin, race.Client().Build(testPkg), "different flags get their own binary")

	CleanupBuildArtifacts()
	require.NoFileExists(t, bin)
}
