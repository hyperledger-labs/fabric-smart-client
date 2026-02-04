/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"path"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/node"
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/node/remote/workload"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"google.golang.org/grpc/benchmark/flags"
	"google.golang.org/grpc/benchmark/stats"
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
	nodeConfPath := path.Join(testdataPath, "fsc", "nodes", "test-node.0")
	clientConfPath := path.Join(nodeConfPath, "client-config.yaml")

	for _, w := range *workloads {
		// select our workload
		var in []byte
		var err error
		for _, v := range workload.Workloads {
			if v.Name == w {
				// Found!
				in, err = json.Marshal(v.Params)
				must(err)
			}
		}

		makeClientF := func(cli *benchmark.ViewClient) func(ctx context.Context) error {
			c := &protos2.Command{
				Payload: &protos2.Command_CallView{CallView: &protos2.CallView{Fid: w, Input: in}},
			}

			sc, err := cli.CreateSignedCommand(c)
			if err != nil {
				panic(err)
			}

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

		for _, nc := range *numConn {
			// create connections
			ccs, cleanup := createClients(nc, clientConfPath)

			// warmup the connections
			err := node.WarmupClients(ccs, makeClientF)
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

					hist := run(ccs, makeClientF, nw, warmDeadline, endDeadline)
					parseHist(hist, *duration)
				}
			}
			// cleanup connections
			cleanup()
		}
	}
}

func run(ccs []*benchmark.ViewClient, makeCaller func(cli *benchmark.ViewClient) func(ctx context.Context) error, numWorker int, warmDeadline, endDeadline time.Time) *stats.Histogram {
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

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func createClients(numConn int, clientConfPath string) ([]*benchmark.ViewClient, func()) {
	ccs := make([]*benchmark.ViewClient, numConn)
	closers := make([]func(), numConn)

	var err error
	for i := 0; i < len(ccs); i++ {
		ccs[i], closers[i], err = node.SetupClient(clientConfPath)
		if err != nil {
			panic(err)
		}
	}

	return ccs, func() {
		for _, c := range closers {
			c()
		}
	}
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
