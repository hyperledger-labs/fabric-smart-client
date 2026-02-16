/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/views"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ViewManager is the minimal interface needed for API benchmarks.
// It is satisfied by the value returned from viewregistry.GetManager().
type ViewManager interface {
	NewView(id string, in []byte) (view.View, error)
	InitiateView(view view.View, ctx context.Context) (interface{}, error)
}

// Workload defines a benchmark workload: a named view factory with optional parameters.
type Workload struct {
	Name    string
	Factory viewregistry.Factory
	Params  any
}

// DefaultWorkloads defines the standard set of benchmark workloads.
var DefaultWorkloads = []Workload{
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

// CreateClients creates numConn ViewClients using the given client config path.
// It returns the clients and a cleanup function that closes all connections.
func CreateClients(tb testing.TB, numConn int, clientConfPath string) ([]*benchmark.ViewClient, func()) {
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

// RunGRPCBenchmark runs a parallel benchmark over multiple ViewClients using
// round-robin caller selection. This is the core gRPC benchmark loop used by
// both local and remote node benchmarks.
func RunGRPCBenchmark(b *testing.B, ccs []*benchmark.ViewClient, makeCaller func(cli *benchmark.ViewClient) func(ctx context.Context) error) {
	callers := make([]func(ctx context.Context) error, len(ccs))
	for i, cc := range ccs {
		callers[i] = makeCaller(cc)
	}

	var rr uint64
	pickCaller := func() func(ctx context.Context) error {
		idx := atomic.AddUint64(&rr, 1)
		return callers[idx%uint64(len(callers))]
	}

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

// MakeGRPCCaller creates a caller function that sends a signed gRPC command
// for the given workload name and input. The signed command is created once
// and reused across all calls for efficiency.
func MakeGRPCCaller(tb testing.TB, cli *benchmark.ViewClient, fid string, input []byte) func(ctx context.Context) error {
	sc, err := cli.CreateSignedCommand(buildCommand(fid, input))
	require.NoError(tb, err)

	return func(ctx context.Context) error {
		return executeSignedCommand(cli, sc, ctx)
	}
}

// buildCommand creates a CallView command for the given view ID and input.
func buildCommand(fid string, input []byte) *protos2.Command {
	return &protos2.Command{
		Payload: &protos2.Command_CallView{CallView: &protos2.CallView{Fid: fid, Input: input}},
	}
}

// executeSignedCommand sends a pre-signed command and validates the response.
func executeSignedCommand(cli *benchmark.ViewClient, sc *protos2.SignedCommand, ctx context.Context) error {
	resp, err := cli.Client.ProcessCommand(ctx, sc)
	if err != nil {
		return err
	}

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

// RunAPIBenchmark runs a direct View API benchmark for a single workload against
// the given view manager. It creates two sub-benchmarks:
//   - f=0: reuses a single view instance per goroutine (measures pure view performance)
//   - f=1: creates a new view per invocation (measures view + factory overhead)
func RunAPIBenchmark(b *testing.B, vm ViewManager, wl Workload) {
	var in []byte
	var err error
	if wl.Params != nil {
		in, err = json.Marshal(wl.Params)
		require.NoError(b, err)
	}

	// warmup
	f, err := vm.NewView(wl.Name, in)
	require.NoError(b, err)
	for range 1000 {
		_, err = vm.InitiateView(f, context.Background())
		assert.NoError(b, err)
	}
	runtime.GC()
	b.ResetTimer()

	// f=0: reuse view instance
	b.Run(fmt.Sprintf("w=%s/f=0/nc=0", wl.Name), func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			f, err := vm.NewView(wl.Name, in)
			assert.NoError(b, err)

			for pb.Next() {
				_, err = vm.InitiateView(f, context.Background())
				assert.NoError(b, err)
			}
		})
		benchmark.ReportTPS(b)
	})

	// f=1: new view per call
	b.Run(fmt.Sprintf("w=%s/f=1/nc=0", wl.Name), func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				f, err := vm.NewView(wl.Name, in)
				assert.NoError(b, err)
				_, err = vm.InitiateView(f, context.Background())
				assert.NoError(b, err)
			}
		})
		benchmark.ReportTPS(b)
	})
}

// RunAPIGRPCBenchmark runs a gRPC View API benchmark for a single workload
// across multiple connection counts. It handles client creation, warmup,
// and cleanup.
func RunAPIGRPCBenchmark(b *testing.B, wl Workload, clientConfPath string, connCounts []int) {
	var in []byte
	var err error
	if wl.Params != nil {
		in, err = json.Marshal(wl.Params)
		require.NoError(b, err)
	}

	for _, nc := range connCounts {
		ccs, cleanup := CreateClients(b, nc, clientConfPath)

		makeClientF := func(cli *benchmark.ViewClient) func(ctx context.Context) error {
			return MakeGRPCCaller(b, cli, wl.Name, in)
		}

		err = WarmupClients(ccs, makeClientF)
		require.NoError(b, err)

		runtime.GC()
		b.ResetTimer()

		b.Run(fmt.Sprintf("w=%s/f=1/nc=%d", wl.Name, nc), func(b *testing.B) {
			RunGRPCBenchmark(b, ccs, makeClientF)
		})

		cleanup()
	}
}
