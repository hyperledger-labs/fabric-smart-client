/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/grpc/remote/workload"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/benchmark/stats"
	"google.golang.org/grpc/credentials"
)

// grpcBufferSize is used to increase the grpc read and write buffer sizes from default 32k to 128k
const grpcBufferSize = 128 * 1024

func FatalF(err error) {
	panic(err)
}

func createConnections(numConn int) ([]*grpc.ClientConn, func()) {
	ccs := make([]*grpc.ClientConn, numConn)
	for i := range ccs {
		conn, err := grpc.NewClient(*addr,
			grpc.WithTransportCredentials(
				credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}),
			),
			grpc.WithWriteBufferSize(grpcBufferSize),
			grpc.WithReadBufferSize(grpcBufferSize),
		)
		if err != nil {
			FatalF(err)
		}
		ccs[i] = conn
	}

	return ccs, func() {
		for _, c := range ccs {
			_ = c.Close()
		}
	}
}

func warmupConnections(ccs []*grpc.ClientConn, makeCaller func(conn *grpc.ClientConn) workload.ClientFunc) {
	g, ctx := errgroup.WithContext(context.Background())

	for _, cc := range ccs {
		conn := cc
		client := makeCaller(conn)
		for range runtime.NumCPU() {
			g.Go(func() error {
				for range 20 {
					err := client(ctx)
					if err != nil {
						return err
					}
				}
				return nil
			})
		}
	}
	if err := g.Wait(); err != nil {
		FatalF(err)
	}
}

func run(ccs []*grpc.ClientConn, makeCaller func(conn *grpc.ClientConn) workload.ClientFunc, numWorker int, warmDeadline, endDeadline time.Time) *stats.Histogram {
	ctx, cancel := context.WithDeadline(context.Background(), endDeadline)
	defer cancel()

	hists := make([]*stats.Histogram, numWorker)

	callers := make([]workload.ClientFunc, len(ccs))
	for i, cc := range ccs {
		callers[i] = makeCaller(cc)
	}

	var rr uint64
	pickCaller := func() workload.ClientFunc {
		idx := atomic.AddUint64(&rr, 1)
		return callers[idx%uint64(len(callers))]
	}

	var wg sync.WaitGroup
	// we create multiple concurrent RPC calls per connection
	for idx := 0; idx < numWorker; idx++ {
		slot := idx

		wg.Add(1)
		go func() {
			defer wg.Done()

			hist := stats.NewHistogram(hopts)
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

	merged := stats.NewHistogram(hopts)
	for _, h := range hists {
		merged.Merge(h)
	}

	return merged
}

var hopts = stats.HistogramOptions{
	NumBuckets:   2495,
	GrowthFactor: .01,
}

func parseHist(hist *stats.Histogram, duration time.Duration) {
	k := testing.BenchmarkResult{N: int(hist.Count), T: duration, Extra: map[string]float64{
		"TPS":         float64(hist.Count) / (duration).Seconds(),
		"ns/op (p5)":  float64(time.Duration(percentile(.5, hist))),
		"ns/op (p95)": float64(time.Duration(percentile(.95, hist))),
		"ns/op (p99)": float64(time.Duration(percentile(.99, hist))),
		"ns/op":       0,
		//"ns/op (_min)": float64(time.Duration(hist.Min)),
		//"ns/op (_max)": float64(time.Duration(hist.Max)),
	}}
	fmt.Println(k.String())
}

func percentile(percentile float64, h *stats.Histogram) int64 {
	need := int64(float64(h.Count) * percentile)
	have := int64(0)
	for _, bucket := range h.Buckets {
		count := bucket.Count
		if have+count >= need {
			percent := float64(need-have) / float64(count)
			return int64((1.0-percent)*bucket.LowBound + percent*bucket.LowBound*(1.0+hopts.GrowthFactor))
		}
		have += bucket.Count
	}
	panic("should have found a bound")
}
