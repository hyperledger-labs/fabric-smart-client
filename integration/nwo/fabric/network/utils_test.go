/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindCmdAtEnv(t *testing.T) { //nolint:paralleltest // t.Setenv
	bins := t.TempDir()
	subdir := filepath.Join(bins, "fabric-x")
	require.NoError(t, os.Mkdir(subdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bins, "configtxgen"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "configtxgen"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(bins, "peer"), nil, 0o600))
	t.Setenv(FabricBinsPathEnvKey, bins)

	// No subdirectory: binaries are resolved in $FAB_BINS itself.
	fabric := &Network{}
	require.Equal(t, filepath.Join(bins, "configtxgen"), fabric.findCmdAtEnv(configtxgenCMD))

	// With a subdirectory, its binaries win over the same names in $FAB_BINS...
	fabricx := &Network{BinSubdir: "fabric-x"}
	require.Equal(t, filepath.Join(subdir, "configtxgen"), fabricx.findCmdAtEnv(configtxgenCMD))

	// ...and $FAB_BINS is not searched as a fallback, so a binary that only the
	// other toolchain ships is reported missing rather than silently reused.
	require.Empty(t, fabricx.findCmdAtEnv(peerCMD))

	t.Setenv(FabricBinsPathEnvKey, "")
	require.Empty(t, fabric.findCmdAtEnv(configtxgenCMD))
}
