/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"path"
	"testing"

	viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/stretchr/testify/require"
)

// BenchmarkAPI exercises the ViewAPI
func BenchmarkAPI(b *testing.B) {
	testdataPath := b.TempDir()
	nodeConfPath := path.Join(testdataPath, "fsc", "nodes", "test-node.0")

	// we generate our testdata
	err := GenerateConfig(testdataPath)
	require.NoError(b, err)
	// create the factories for we register with our node server
	fcs := make([]NamedFactory, len(DefaultWorkloads))
	for i, bm := range DefaultWorkloads {
		fcs[i] = NamedFactory{
			Name:    bm.Name,
			Factory: bm.Factory,
		}
	}

	// create server
	n, err := SetupNode(nodeConfPath, fcs...)
	require.NoError(b, err)

	vm, err := viewregistry.GetManager(n)
	require.NoError(b, err)

	// run all workloads via direct view API
	for _, bm := range DefaultWorkloads {
		RunAPIBenchmark(b, vm, bm)
	}

	n.Stop()
}
