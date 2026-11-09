/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReleaseGoMod checks that a release build pins every FSC module the
// generated main depends on, by version and by checkout directory.
func TestReleaseGoMod(t *testing.T) {
	t.Parallel()

	byVersion := map[string]string{}
	for _, m := range releaseModules {
		byVersion[m] = "v0.28.0"
	}
	byDir, err := releaseTargets("/src/fsc")
	require.NoError(t, err)

	assert.Equal(t, `module fscrelease

require (
	github.com/hyperledger-labs/fabric-smart-client v0.28.0
	github.com/hyperledger-labs/fabric-smart-client/integration v0.28.0
)

replace (
	github.com/hyperledger-labs/fabric-smart-client => github.com/hyperledger-labs/fabric-smart-client v0.28.0
	github.com/hyperledger-labs/fabric-smart-client/integration => github.com/hyperledger-labs/fabric-smart-client/integration v0.28.0
	github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/cc/query => github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/cc/query v0.28.0
	github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p => github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p v0.28.0
)
`, releaseGoMod(byVersion))

	assert.Contains(t, releaseGoMod(byDir), "\tgithub.com/hyperledger-labs/fabric-smart-client v0.0.0-00010101000000-000000000000\n")
	assert.Contains(t, releaseGoMod(byDir), "\tgithub.com/hyperledger-labs/fabric-smart-client/integration => /src/fsc/integration\n")
	assert.Contains(t, releaseGoMod(byDir), "\tgithub.com/hyperledger-labs/fabric-smart-client => /src/fsc\n")
}

// TestCheckRelease checks that a release names a module version or an absolute
// checkout path, never a relative path, which would resolve against the suite's
// directory.
func TestCheckRelease(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{"v0.28.0", "457266241", "/src/fsc"} {
		require.NoError(t, checkRelease(ok), ok)
	}
	for _, bad := range []string{"../fsc", "./fsc", "src/fsc"} {
		require.ErrorContains(t, checkRelease(bad), "absolute path", bad)
	}
}
