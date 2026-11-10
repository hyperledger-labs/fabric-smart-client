# Benchmarking custom views tutorial

This guide shows how to benchmark a custom `view.View` implementation using FSC's exported benchmark infrastructure from an external repository.

## What you implement

Your view must satisfy two interfaces from FSC:
- [`view.View`](../../../platform/view/view/view.go) : the view logic itself
- [`viewregistry.Factory`](../../../platform/view/services/view/registry.go) : creates new instances of your view

```go
// your_view.go

type YourView struct{ params YourParams }

func (v *YourView) Call(viewCtx view.Context) (any, error) {
    // your workload logic (e.g. ZKP proving, signing, validation)
    return result, nil
}

type YourViewFactory struct{}

func (f *YourViewFactory) NewView(in []byte) (view.View, error) {
    v := &YourView{}
    if in != nil {
        json.Unmarshal(in, &v.params)
    }
    return v, nil
}
```

## File structure

```text
your-repo/
├── your_view.go                        # YourView + YourViewFactory
├── bench/
│   ├── standalone_bench_test.go        # pure view benchmark (no FSC node)
│   ├── node_bench_test.go              # local node benchmarks (API + gRPC)
│   └── remote/
│       ├── server/main.go              # remote server
│       └── client/main.go              # remote client
```

- `your_view.go` : your `View` and `ViewFactory` implementations.
- `standalone_bench_test.go` : benchmarks that call your view directly, no FSC infrastructure.
- `node_bench_test.go` : benchmarks that run your view through a real FSC node (API and gRPC).
- `remote/server/main.go` and `remote/client/main.go` : binaries for running benchmarks across two machines.

## Define your workload

Wrap your view in a [`node.Workload`](./bench_helpers.go) to plug into FSC's benchmark runners:

```go
var yourWorkload = node.Workload{
    Name:    "your-view",
    Factory: &YourViewFactory{},
    Params:  &YourParams{Size: 1024},
}
```

## Benchmark levels

### 1. Standalone - pure view performance

No FSC node, no gRPC. Directly benchmarks `Call()`.

```go
// standalone_bench_test.go

func BenchmarkYourView(b *testing.B) {
    f := &YourViewFactory{}
    input, _ := json.Marshal(&YourParams{Size: 1024})

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
go test -bench=BenchmarkYourView -benchmem -count=5 -cpu=1,2,4,8 ./bench/
```

### 2. Local node - View API and gRPC

Spins up an FSC node in-process using [`node.GenerateConfig`](./benchmark.go) and [`node.SetupNode`](./benchmark.go).

[`RunAPIBenchmark`](./bench_helpers.go) produces two sub-benchmarks:
- **f=0** : creates one view instance per goroutine and reuses it. Measures pure view execution cost.
- **f=1** : creates a new view via the factory on every call. Measures view execution + factory overhead.

[`RunAPIGRPCBenchmark`](./bench_helpers.go) adds the full gRPC stack on top and tests across multiple connection counts.

```go
// node_bench_test.go

func BenchmarkYourViewAPI(b *testing.B) {
    testdataPath := b.TempDir()
    nodeConfPath := path.Join(testdataPath, "fsc", "nodes", "test-node.0")

    require.NoError(b, node.GenerateConfig(testdataPath))

    n, err := node.SetupNode(nodeConfPath, node.NamedFactory{
        Name:    yourWorkload.Name,
        Factory: yourWorkload.Factory,
    })
    require.NoError(b, err)
    defer n.Stop()

    vm, err := viewregistry.GetManager(n)
    require.NoError(b, err)

    node.RunAPIBenchmark(b, vm, yourWorkload)
}

func BenchmarkYourViewAPIGRPC(b *testing.B) {
    testdataPath := b.TempDir()
    nodeConfPath := path.Join(testdataPath, "fsc", "nodes", "test-node.0")
    clientConfPath := path.Join(nodeConfPath, "client-config.yaml")

    require.NoError(b, node.GenerateConfig(testdataPath))

    n, err := node.SetupNode(nodeConfPath, node.NamedFactory{
        Name:    yourWorkload.Name,
        Factory: yourWorkload.Factory,
    })
    require.NoError(b, err)
    defer n.Stop()

    node.RunAPIGRPCBenchmark(b, yourWorkload, clientConfPath, *numConn)
}
```

```bash
go test -bench=BenchmarkYourViewAPI -benchmem -count=5 -cpu=1,2,4,8 ./bench/
go test -bench=BenchmarkYourViewAPIGRPC -benchmem -count=5 -cpu=1,8,16 -numConn=1,2,4 ./bench/
```

### 3. Remote - two machines

**Server** - register your factory and start the node:

```go
// remote/server/main.go

func main() {
    testdataPath := "./out/testdata"
    nodeConfPath := path.Join(testdataPath, "fsc", "nodes", "test-node.0")

    if err := node.GenerateConfig(testdataPath); err != nil {
        panic(err)
    }

    n, err := node.SetupNode(nodeConfPath,
        node.NamedFactory{Name: "your-view", Factory: &YourViewFactory{}},
    )
    if err != nil {
        panic(err)
    }

    // wait for signal...
}
```

**Client** - populate config and call [`RunRemoteBenchmarkSuite`](./bench_remote.go):

```go
// remote/client/main.go

func main() {
    flag.Parse()
    node.RunRemoteBenchmarkSuite(node.RemoteBenchmarkConfig{
        Workloads:      []node.Workload{yourWorkload},
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

#### Reference

FSC's own remote benchmarks use the same infrastructure:
- [Server](./remote/server/main.go)
- [Client](./remote/client/main.go)

#### Deployment

When the server starts, `node.GenerateConfig` generates all required crypto material and configuration into `./out/testdata`, including TLS certificates used by both server and client. This folder must be copied to the client machine before running benchmarks. The client currently expects it at `./out/testdata` this path is not yet configurable.

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
| API f=0 | ViewManager + reused view | [`node.RunAPIBenchmark`](./bench_helpers.go) |
| API f=1 | ViewManager + factory per call | [`node.RunAPIBenchmark`](./bench_helpers.go) |
| gRPC (local) | Full FSC gRPC stack | [`node.RunAPIGRPCBenchmark`](./bench_helpers.go) |
| gRPC (remote) | Full stack + real network | [`node.RunRemoteBenchmarkSuite`](./bench_remote.go) |






