/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	"google.golang.org/grpc/benchmark/stats"
)

// HistogramOptions are the default bucket settings for latency histograms
// used by deadline-driven (remote) benchmarks.
var HistogramOptions = stats.HistogramOptions{
	NumBuckets:   2495,
	GrowthFactor: .01,
}

// MakeRemoteGRPCCaller creates a caller function for deadline-driven (remote)
// benchmarks. Unlike MakeGRPCCaller (which takes testing.TB), this version
// panics on setup errors since it runs outside of `go test`.
func MakeRemoteGRPCCaller(cli *benchmark.ViewClient, fid string, input []byte) func(ctx context.Context) error {
	sc, err := cli.CreateSignedCommand(buildCommand(fid, input))
	if err != nil {
		panic(fmt.Sprintf("failed to create signed command: %v", err))
	}

	return func(ctx context.Context) error {
		return executeSignedCommand(cli, sc, ctx)
	}
}

// RunRemoteBenchmark runs a deadline-driven benchmark across multiple ViewClients.
// Unlike RunGRPCBenchmark (which uses testing.B), this uses explicit warmup and
// measurement phases with histogram-based latency collection.
//
// It returns the merged histogram of all workers. Callers typically pass the
// result to PrintHistogram().
func RunRemoteBenchmark(
	ccs []*benchmark.ViewClient,
	makeCaller func(cli *benchmark.ViewClient) func(ctx context.Context) error,
	numWorker int,
	warmDeadline, endDeadline time.Time,
) *stats.Histogram {
	ctx, cancel := context.WithDeadline(context.Background(), endDeadline)
	defer cancel()

	hists := make([]*stats.Histogram, numWorker)

	callers := make([]func(ctx context.Context) error, len(ccs))
	for i, cc := range ccs {
		callers[i] = makeCaller(cc)
	}

	var rr uint64
	pickCaller := func() func(ctx context.Context) error {
		idx := atomic.AddUint64(&rr, 1)
		return callers[idx%uint64(len(callers))]
	}

	var wg sync.WaitGroup
	for idx := 0; idx < numWorker; idx++ {
		slot := idx

		wg.Add(1)
		go func() {
			defer wg.Done()

			hist := stats.NewHistogram(HistogramOptions)
			for ctx.Err() == nil {
				caller := pickCaller()

				start := time.Now()
				err := caller(ctx)
				elapsed := time.Since(start)

				if err == nil && start.After(warmDeadline) {
					_ = hist.Add(elapsed.Nanoseconds())
				}
			}

			hists[slot] = hist
		}()
	}
	wg.Wait()

	merged := stats.NewHistogram(HistogramOptions)
	for _, h := range hists {
		merged.Merge(h)
	}

	return merged
}

// PrintHistogram formats and prints benchmark results from a histogram,
// including TPS and latency percentiles (p5, p95, p99).
//
// Output format matches Go's testing.BenchmarkResult so it can be parsed
// by standard benchmark tooling.
func PrintHistogram(hist *stats.Histogram, duration time.Duration) {
	k := testing.BenchmarkResult{N: int(hist.Count), T: duration, Extra: map[string]float64{
		"TPS":         float64(hist.Count) / duration.Seconds(),
		"ns/op (p5)":  float64(time.Duration(Percentile(.5, hist))),
		"ns/op (p95)": float64(time.Duration(Percentile(.95, hist))),
		"ns/op (p99)": float64(time.Duration(Percentile(.99, hist))),
		"ns/op":       0,
	}}
	fmt.Println(k.String())
}

// Percentile computes the given percentile (0.0–1.0) from a histogram.
func Percentile(p float64, h *stats.Histogram) int64 {
	need := int64(float64(h.Count) * p)
	have := int64(0)
	for _, bucket := range h.Buckets {
		count := bucket.Count
		if have+count >= need {
			percent := float64(need-have) / float64(count)
			return int64((1.0-percent)*bucket.LowBound + percent*bucket.LowBound*(1.0+HistogramOptions.GrowthFactor))
		}
		have += bucket.Count
	}
	panic("should have found a bound")
}

// CreateRemoteClients creates numConn ViewClients for remote benchmarks.
// Unlike CreateClients (which takes testing.TB), this panics on errors
// since it runs outside of `go test`.
func CreateRemoteClients(numConn int, clientConfPath string) ([]*benchmark.ViewClient, func()) {
	ccs := make([]*benchmark.ViewClient, numConn)
	closers := make([]func(), numConn)

	for i := 0; i < numConn; i++ {
		var err error
		ccs[i], closers[i], err = SetupClient(clientConfPath)
		if err != nil {
			panic(fmt.Sprintf("failed to create client %d: %v", i, err))
		}
	}

	return ccs, func() {
		for _, c := range closers {
			c()
		}
	}
}

// MarshalWorkloadParams marshals workload params to JSON.
// Returns nil if params is nil.
func MarshalWorkloadParams(params any) []byte {
	if params == nil {
		return nil
	}
	in, err := json.Marshal(params)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal workload params: %v", err))
	}
	return in
}
