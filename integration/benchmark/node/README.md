# Benchmarking Custom Views Tutorial

This guide shows how to benchmark a custom `view.View` implementation using FSC's exported benchmark infrastructure from an external repository.

## What You Implement

Your view must satisfy two interfaces from FSC:

```go
// view.View
type ZKPSignView struct{ ... }
func (z *ZKPSignView) Call(ctx view.Context) (interface{}, error) { ... }

// viewregistry.Factory
type ZKPSignViewFactory struct{}
func (f *ZKPSignViewFactory) NewView(in []byte) (view.View, error) { ... }
```

Define your workload using the exported `node.Workload` type:

```go
var zkpWorkload = node.Workload{
    Name:    "zkp",
    Factory: &zkp.ZKPSignViewFactory{},
    Params:  &zkp.ZKPSignParams{CircuitSize: 1024},
}
```

## Benchmark Levels

### 1. Standalone - Pure View Performance

No FSC node, no gRPC. Directly benchmarks `Call()`.

```go
func BenchmarkZKPSign(b *testing.B) {
    f := &zkp.ZKPSignViewFactory{}
    input, _ := json.Marshal(&zkp.ZKPSignParams{CircuitSize: 1024})

    b.RunParallel(func(pb *testing.PB) {
        v, _ := f.NewView(input)
        for pb.Next() {
            _, _ = v.Call(nil)
        }
    })
    benchmark.ReportTPS(b)
}
```

```bash
go test -bench=BenchmarkZKPSign -benchmem -count=5 -cpu=1,2,4,8 ./bench/
```

### 2. Local Node - View API and gRPC

Spins up an FSC node in process. `RunAPIBenchmark` produces f=0 (reuse view) and f=1 (new view per call) benchmarks. `RunAPIGRPCBenchmark` tests across connection counts.

```go
func BenchmarkZKPAPI(b *testing.B) {
    // ... setup node with node.GenerateConfig + node.SetupNode ...
    vm, _ := viewregistry.GetManager(n)
    node.RunAPIBenchmark(b, vm, zkpWorkload)
}

func BenchmarkZKPAPIGRPC(b *testing.B) {
    // ... setup node ...
    node.RunAPIGRPCBenchmark(b, zkpWorkload, clientConfPath, *numConn)
}
```

```bash
go test -bench=BenchmarkZKPAPI -benchmem -count=5 -cpu=1,2,4,8 ./bench/
go test -bench=BenchmarkZKPAPIGRPC -benchmem -count=5 -cpu=1,8,16 -numConn=1,2,4 ./bench/
```

### 3. Remote - Two Machines

**Server** - register your factory and start the node:

```go
func main() {
    node.GenerateConfig(testdataPath)
    n, _ := node.SetupNode(nodeConfPath,
        node.NamedFactory{Name: "zkp", Factory: &zkp.ZKPSignViewFactory{}},
    )
    // wait for signal...
}
```

**Client** - populate config and call `RunRemoteBenchmarkSuite`:

```go
func main() {
    flag.Parse()
    node.RunRemoteBenchmarkSuite(node.RemoteBenchmarkConfig{
        Workloads:      []node.Workload{zkpWorkload},
        ClientConfPath: clientConfPath,
        ConnCounts:     *numConn,
        WorkerCounts:   *numWorker,
        WarmupDur:      *warmupDur,
        BenchTime:      *duration,
        Count:          *count,
        BenchName:      "BenchmarkRemote",
    })
}
```
#### Reference - FSC's own remote benchmarks use the same infrastructure:
- [Server](integration/benchmark/node/remote/server/main.go)
- [Client](integration/benchmark/node/remote/client/main.go)



**Deployment:**

The server generates all crypto material and configuration (including TLS certificates for both server and client) into `./out/testdata`. This folder must be copied to the client machine before running benchmarks. For simplicity we rsync the entire testdata folder. Note that the client currently expects the testdata at `./out/testdata` — this path is not yet configurable.

```bash
# server machine
go run ./bench/remote/server/

# copy crypto material to client
rsync -av server:/path/to/out/testdata ./out/testdata
vim ./out/testdata/fsc/nodes/test-node.0/client-config.yaml  # update server address

# client machine
go run ./bench/remote/client/ \
  -benchtime=10s -count=5 -cpu=1,8,16,32 -numConn=1,2,4 2>&1 | tee results.txt
```

## Summary

| Level | What it measures | How to run |
|---|---|---|
| Standalone | Pure `Call()` | `go test -bench` |
| API f=0 | ViewManager + reused view | `node.RunAPIBenchmark` |
| API f=1 | ViewManager + factory per call | `node.RunAPIBenchmark` |
| gRPC (local) | Full FSC gRPC stack | `node.RunAPIGRPCBenchmark` |
| gRPC (remote) | Full stack + real network | `node.RunRemoteBenchmarkSuite` |