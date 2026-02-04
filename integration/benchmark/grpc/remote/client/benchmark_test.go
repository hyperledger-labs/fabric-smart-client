/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/grpc/remote/workload"
	"google.golang.org/grpc"
)

func BenchmarkLocal(b *testing.B) {
	for _, w := range *workloads {
		// select our workload
		makeClientF := workloadTypes[w]

		for _, nc := range *numConn {
			// create connections
			ccs, cleanup := createConnections(nc)

			// warmup the connections
			warmupConnections(ccs, makeClientF)
			runtime.GC()

			b.Run(fmt.Sprintf("w=%v/nc=%d", w, nc), func(b *testing.B) { runBenchmark(b, ccs, makeClientF) })

			// cleanup connections
			cleanup()
		}
	}
}

func runBenchmark(b *testing.B, ccs []*grpc.ClientConn, makeCaller func(conn *grpc.ClientConn) workload.ClientFunc) {
	callers := make([]workload.ClientFunc, len(ccs))
	for i, cc := range ccs {
		callers[i] = makeCaller(cc)
	}

	var rr uint64
	pickCaller := func() workload.ClientFunc {
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
	ReportTPS(b)
}

func ReportTPS(b *testing.B) {
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "TPS")
}
