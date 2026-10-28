/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const cliPackagePath = "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/fsccli"

// TestArtifactsGen builds the CLI, so it also covers the CLI compiling.
func TestArtifactsGen(t *testing.T) {
	t.Parallel()

	cli := filepath.Join(t.TempDir(), "fsccli")
	out, err := exec.Command("go", "build", "-o", cli, cliPackagePath).CombinedOutput()
	require.NoError(t, err, string(out))

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	outDir := t.TempDir()
	topology := filepath.Join("testdata", "fabric_iou.yaml")
	cmd := exec.CommandContext(ctx, cli, "artifactsgen", "gen", "-t", topology, "-o", outDir)
	// The CLI starts child processes that share its output pipe; without a
	// WaitDelay, one that outlives the timeout would block CombinedOutput.
	cmd.WaitDelay = 5 * time.Second
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)

	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	require.Equal(t, []string{"conf.json", "fabric.default", "fsc", "topology.yaml"}, names)
}
