/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"path"
	"runtime"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/node"
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/node/remote/workload"
	"google.golang.org/grpc/benchmark/flags"
)

var (
	numConn   = flags.IntSlice("numConn", []int{1, 2}, "Number of grpc client connections - may be a comma-separated list")
	numWorker = flags.IntSlice("cpu", []int{1, 2, 4, 8}, "Number of concurrent worker - may be a comma-separated list")
	workloads = flags.StringSlice("workloads", []string{"sign"}, "Workloads to execute - may be a comma-separated list")
	warmupDur = flag.Duration("warmup", 5*time.Second, "Warmup duration")
	duration  = flag.Duration("benchtime", 10*time.Second, "Duration for every execution")
	count     = flag.Int("count", 1, "Number of executions")
)

func main() {
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	testdataPath := "./out/testdata" // for local debugging you can set testdataPath := "out/testdata"
	clientConfPath := path.Join(testdataPath, "fsc", "nodes", "test-node.0", "client-config.yaml")

	for _, w := range *workloads {
		// find our workload
		var wl node.Workload
		for _, v := range workload.Workloads {
			if v.Name == w {
				wl = v
				break
			}
		}

		in := node.MarshalWorkloadParams(wl.Params)

		makeCaller := func(cli *benchmark.ViewClient) func(ctx context.Context) error {
			return node.MakeRemoteGRPCCaller(cli, wl.Name, in)
		}

		for _, nc := range *numConn {
			// create connections
			ccs, cleanup := node.CreateRemoteClients(nc, clientConfPath)

			// warmup the connections
			err := node.WarmupClients(ccs, makeCaller)
			if err != nil {
				panic(err)
			}
			runtime.GC()

			fmt.Printf("BenchmarkAPIGRPCRemote/w=%v/nc=%d\n", w, nc)
			// worker
			for _, nw := range *numWorker {
				// count
				for range *count {
					fmt.Printf("BenchmarkAPIGRPCRemote/w=%v/nc=%d-%v\t\t ", w, nc, nw)

					warmDeadline := time.Now().Add(*warmupDur)
					endDeadline := warmDeadline.Add(*duration)

					hist := node.RunRemoteBenchmark(ccs, makeCaller, nw, warmDeadline, endDeadline)
					node.PrintHistogram(hist, *duration)
				}
			}
			// cleanup connections
			cleanup()
		}
	}
}
