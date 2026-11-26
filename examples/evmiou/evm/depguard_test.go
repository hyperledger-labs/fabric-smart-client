/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Copied from Panurus x/token/services/network/evm at f144b2b7f.

package evm_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// forbiddenDependency is the module that must never enter the EVM client's build graph.
// Its license (LGPL) is a hard blocker for the project, so all Ethereum primitives (hashing,
// addresses, signing, RLP) are provided locally or by permissively-licensed libraries.
const forbiddenDependency = "github.com/ethereum/go-ethereum"

// TestNoGoEthereumDependency fails if go-ethereum appears anywhere in the transitive dependencies
// of the EVM client packages. It is intentionally a build-graph guard rather than a unit test: it
// catches an accidental import the moment it lands, in CI, across every current and future file.
func TestNoGoEthereumDependency(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-deps", "./...")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("skipping: could not run 'go list -deps' (%v): %s", err, out)
	}
	for dep := range strings.SplitSeq(string(out), "\n") {
		require.NotContains(t, dep, forbiddenDependency,
			"go-ethereum must not be linked (license blocker); found it in the build graph")
	}
}
