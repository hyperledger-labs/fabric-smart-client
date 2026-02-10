/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"flag"
	"fmt"
	"runtime"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/grpc/remote/workload"
	"google.golang.org/grpc"
	"google.golang.org/grpc/benchmark/flags"
)

var (
	addr      = flag.String("addr", "localhost:8099", "Server endpoint")
	numConn   = flags.IntSlice("numConn", []int{1, 2}, "Number of grpc client connections - may be a comma-separated list")
	numWorker = flags.IntSlice("cpu", []int{1, 2, 4, 8}, "Number of concurrent worker - may be a comma-separated list")
	workloads = flags.StringSlice("workloads", []string{workload.ECDSA}, "Workloads to execute - may be a comma-separated list")
	warmupDur = flag.Duration("warmup", 5*time.Second, "Warmup duration")
	duration  = flag.Duration("benchtime", 10*time.Second, "Duration for every execution")
	count     = flag.Int("count", 1, "Number of executions")
)

var workloadTypes = map[string]func(conn *grpc.ClientConn) workload.ClientFunc{
	workload.NOOP:  workload.CreateClientFuncNoop, // this is just a local noop; no grpc
	workload.ECHO:  workload.CreateClientFuncEcho,
	workload.ECDSA: workload.CreateClientFuncECDSA,
	workload.CPU:   workload.CreateClientFuncCPU,
}

func main() {
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	for _, w := range *workloads {
		// select our workload
		makeClientF := workloadTypes[w]

		for _, nc := range *numConn {
			// create connections
			ccs, cleanup := createConnections(nc)

			// warmup the connections
			warmupConnections(ccs, makeClientF)
			runtime.GC()

			fmt.Printf("BenchmarkRemote/w=%v/nc=%d\n", w, nc)
			// worker
			for _, nw := range *numWorker {
				// count
				for range *count {
					fmt.Printf("benchmarkRemote/w=%v/nc=%d-%v\t\t ", w, nc, nw)

					warmDeadline := time.Now().Add(*warmupDur)
					endDeadline := warmDeadline.Add(*duration)

					hist := run(ccs, makeClientF, nw, warmDeadline, endDeadline)
					parseHist(hist, *duration)
				}
			}
			// cleanup connections
			cleanup()
		}
	}
}
