/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"runtime"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/views"
	viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var workloads = []struct {
	name    string
	factory viewregistry.Factory
	params  any
}{
	{
		name:    "noop",
		factory: &views.NoopViewFactory{},
	},
	{
		name:    "cpu",
		factory: &views.CPUViewFactory{},
		params:  &views.CPUParams{N: 200000},
	},
	{
		name:    "sign",
		factory: &views.ECDSASignViewFactory{},
		params:  &views.ECDSASignParams{},
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
			Name:    bm.name,
			Factory: bm.factory,
		}
	}

	// create server
	n, err := SetupNode(nodeConfPath, fcs...)
	require.NoError(b, err)

	vm, err := viewregistry.GetManager(n)
	require.NoError(b, err)

	// run all workloads via direct view API
	for _, bm := range workloads {
		// select our workload

		var in []byte
		if bm.params != nil {
			in, err = json.Marshal(bm.params)
			require.NoError(b, err)
		}

		// warmup node
		f, err := vm.NewView(bm.name, in)
		for range 1000 {
			_, err = vm.InitiateView(f, context.Background())
			assert.NoError(b, err)
		}
		runtime.GC()
		b.ResetTimer()

		// this benchmark calls the view directly via view API
		// note that every worker first creates a view instance using the Factory and then invokes it
		b.Run(fmt.Sprintf("w=%s/f=0/nc=0", bm.name), func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				// each goroutine instantiates a dedicated view
				f, err := vm.NewView(bm.name, in)
				assert.NoError(b, err)

				for pb.Next() {
					_, err = vm.InitiateView(f, context.Background())
					assert.NoError(b, err)
				}
			})
			benchmark.ReportTPS(b)
		})

		// this benchmark calls the view directly via view API
		// note that every invocation also calls the factor to create a fresh view instance
		b.Run(fmt.Sprintf("w=%s/f=1/nc=0", bm.name), func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				// each goroutine instantiates a dedicated view
				assert.NoError(b, err)

				for pb.Next() {
					f, err := vm.NewView(bm.name, in)
					assert.NoError(b, err)
					_, err = vm.InitiateView(f, context.Background())
					assert.NoError(b, err)
				}
			})
			benchmark.ReportTPS(b)
		})
	}
	// cleanup server
	n.Stop()
}
