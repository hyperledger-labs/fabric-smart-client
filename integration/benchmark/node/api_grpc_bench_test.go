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
	"sync/atomic"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
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

	// run all workloads via direct view API
	for _, bm := range workloads {
		// select our workload

		for _, nc := range *numConn {
			// create connections
			ccs, cleanup := createClients(b, nc, clientConfPath)

			// our payload
			var in []byte
			if bm.params != nil {
				in, err = json.Marshal(bm.params)
				require.NoError(b, err)
			}

			// define our work
			makeClientF := func(cli *benchmark.ViewClient) func(ctx context.Context) error {
				c := &protos2.Command{
					Payload: &protos2.Command_CallView{CallView: &protos2.CallView{Fid: bm.name, Input: in}},
				}

				sc, err := cli.CreateSignedCommand(c)
				require.NoError(b, err)

				return func(ctx context.Context) error {
					resp, err := cli.Client.ProcessCommand(ctx, sc)
					if err != nil {
						return err
					}

					// extract error messages from the response
					commandResp := &protos2.CommandResponse{}
					err = proto.Unmarshal(resp.Response, commandResp)
					if err != nil {
						return err
					}
					if commandResp.GetErr() != nil {
						return fmt.Errorf("error from view during process command: %s", commandResp.GetErr().GetMessage())
					}

					return nil
				}
			}

			err = WarmupClients(ccs, makeClientF)
			require.NoError(b, err)

			runtime.GC()
			b.ResetTimer()

			b.Run(fmt.Sprintf("w=%s/f=1/nc=%d", bm.name, nc), func(b *testing.B) { runBenchmark(b, ccs, makeClientF) })
			// cleanup clients
			cleanup()
		}
	}
	// cleanup server
	n.Stop()
}

func createClients(tb testing.TB, numConn int, clientConfPath string) ([]*benchmark.ViewClient, func()) {
	ccs := make([]*benchmark.ViewClient, numConn)
	closers := make([]func(), numConn)

	var err error
	for i := 0; i < len(ccs); i++ {
		ccs[i], closers[i], err = SetupClient(clientConfPath)
		require.NoError(tb, err)
	}

	return ccs, func() {
		for _, c := range closers {
			c()
		}
	}
}

func runBenchmark(b *testing.B, ccs []*benchmark.ViewClient, makeCaller func(cli *benchmark.ViewClient) func(ctx context.Context) error) {
	callers := make([]func(ctx context.Context) error, len(ccs))
	for i, cc := range ccs {
		callers[i] = makeCaller(cc)
	}

	var rr uint64
	pickCaller := func() func(ctx context.Context) error {
		idx := atomic.AddUint64(&rr, 1)
		return callers[idx%uint64(len(callers))]
	}

	// run our benchmark
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			caller := pickCaller()
			if err := caller(b.Context()); err != nil {
				b.Errorf("err %v", err)
			}
		}
	})
	benchmark.ReportTPS(b)
}
