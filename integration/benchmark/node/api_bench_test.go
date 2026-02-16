/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"path"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/views"
	viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/stretchr/testify/require"
)

var workloads = []Workload{
	{
		Name:    "noop",
		Factory: &views.NoopViewFactory{},
	},
	{
		Name:    "cpu",
		Factory: &views.CPUViewFactory{},
		Params:  &views.CPUParams{N: 200000},
	},
	{
		Name:    "sign",
		Factory: &views.ECDSASignViewFactory{},
		Params:  &views.ECDSASignParams{},
	},
}

// BenchmarkAPI exercises the ViewAPI
func BenchmarkAPI(b *testing.B) {
	testdataPath := b.TempDir() // for local debugging you can set testdataPath := "out/testdata"
	nodeConfPath := path.Join(testdataPath, "fsc", "nodes", "test-node.0")
	//clientConfPath := path.Join(nodeConfPath, "client-config.yaml")

	// we generate our testdata
	err := GenerateConfig(testdataPath)
	require.NoError(b, err)

	// create the factories for we register with our node server
	fcs := make([]NamedFactory, len(workloads))
	for i, bm := range workloads {
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
	for _, bm := range workloads {
		RunAPIBenchmark(b, vm, bm)
	}

	n.Stop()
}
