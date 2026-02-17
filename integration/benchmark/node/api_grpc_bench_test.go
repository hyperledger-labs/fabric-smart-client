/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/benchmark/flags"
)

var numConn = flags.IntSlice("numConn", []int{1, 2, 4, 8}, "Number of grpc client connections - may be a comma-separated list")

// BenchmarkAPIGRPC exercises the ViewAPI via grpc client
func BenchmarkAPIGRPC(b *testing.B) {
	testdataPath := b.TempDir() // for local debugging you can set testdataPath := "out/testdata"
	nodeConfPath := path.Join(testdataPath, "fsc", "nodes", "test-node.0")
	clientConfPath := path.Join(nodeConfPath, "client-config.yaml")

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

	// run all workloads via direct view API
	for _, bm := range DefaultWorkloads {
		RunAPIGRPCBenchmark(b, bm, clientConfPath, *numConn)
	}

	n.Stop()
}
