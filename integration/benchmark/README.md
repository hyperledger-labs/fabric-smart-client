# FSC Benchmarks

Performance benchmarks for the Fabric Smart Client framework, measuring throughput (TPS) and latency across different workloads and deployment scenarios.

## Overview

This package contains micro-benchmarks organized into three categories:

### 1. View Workloads (`views/`)
Standalone view implementations to define various workloads:
- **noop**: Minimal no-op view (baseline overhead)
- **cpu**: CPU-intensive workload (200k iterations)
- **ecdsa**: ECDSA P-256 signing operations

### 2. gRPC Benchmarks (`grpc/`)
Direct gRPC client/server benchmarks without FSC node overhead:
- **grpc/**: Basic gRPC benchmarks with mock/real signers (local)
- **grpc/remote/**: Advanced benchmarks with configurable workloads, connections, and workers
  - Supports local and remote deployment
  - Measures TPS and tail latency (p5, p95, p99)
  - See `grpc/remote/README.md` for details

### 3. Node Benchmarks (`node/`)
FSC node benchmarks via View API and gRPC View API:
- **Direct API**: View execution without network overhead
- **gRPC API**: View execution via gRPC (single/multi-connection)
  - **node/**: Local client/server benchmarks 
  - **node/remote/**: Remote client/server benchmarks
    - See `node/remote/README.md` for details

## Quick Start

### Run View Benchmarks
```bash
# Run all view benchmarks
go test -bench=. -benchmem -count=10 -cpu=1,2,4,8 ./views/

# Run specific workload
go test -bench=BenchmarkECDSA -benchmem -count=10 -cpu=1,2,4,8 ./views/
```

### Run Node Benchmarks
```bash
# Run all node benchmarks
go test -bench=. -benchmem -count=5 -cpu=1,8,16 ./node/

# Run with specific connection counts
go test -bench=BenchmarkAPIGRPC -benchmem -count=5 -cpu=1,8,16 -numConn=1,2,4 ./node/
```

### Run gRPC Benchmarks (Next)
```bash
# Start server
cd grpc/remote/server && go run main.go

# Run client (in another terminal)
cd grpc/remote/client
go run main.go -addr=localhost:8099 -benchtime=10s -count=5 \
  -workloads=echo,cpu,ecdsa -cpu=1,8,16,32 -numConn=1,2,4
```

## Garbage Collection Tuning

Go's garbage collector can significantly impact performance. Test with different settings:

```bash
# Default GC
GOGC=100 go test -bench=. -benchmem -count=10 ./...

# Aggressive GC (more frequent collections)
GOGC=50 go test -bench=. -benchmem -count=10 ./...

# Relaxed GC (less frequent collections)
GOGC=8000 go test -bench=. -benchmem -count=10 ./...

# Disable GC (testing only!)
GOGC=off go test -bench=. -benchmem -count=10 ./...
```

Save results for comparison:
```bash
GOGC=100 go test -bench=. -benchmem -count=10 -cpu=1,2,4,8 ./... > plots/benchmark_gc_100.txt
GOGC=off go test -bench=. -benchmem -count=10 -cpu=1,2,4,8 ./... > plots/benchmark_gc_off.txt
```

## Visualization

The `plots/` directory contains Python scripts to visualize benchmark results:

```bash
# Setup
cd plots
python3 -m venv env
source env/bin/activate
pip3 install -r requirements.txt

# Generate plots
python3 plot.py benchmark_gc_100.txt benchmark_gc_100.pdf
python3 plot_grpc.py grpc_results.txt grpc_results.pdf
python3 plot_node.py node_results.txt node_results.pdf

# Plot all results
./plot_all.sh
```

## Understanding Results

Benchmark output includes:
- **N**: Number of operations executed
- **TPS**: Transactions per second (throughput)
- **ns/op**: Nanoseconds per operation (latency)
- **p5/p95/p99**: Latency percentiles (for next/ benchmarks)
- **B/op**: Bytes allocated per operation
- **allocs/op**: Allocations per operation

Example output:
```
BenchmarkAPIGRPC/w=ecdsa/nc=2-16    236702    23670 TPS    654140 ns/op (p5)    847374 ns/op (p95)
```

## Best Practices

1. **Warmup**: Benchmarks include warmup phases to reach steady-state
2. **Multiple Runs**: Use `-count=5` or higher for statistical significance
3. **CPU Scaling**: Test with `-cpu=1,2,4,8,16` to measure scalability
4. **GC Impact**: Compare results with different GOGC settings
5. **Isolation**: Run on idle systems for consistent results
6. **Remote Testing**: Use `node/remote/` or `grpc/remote/` for realistic network conditions

## Resources

- [Go Benchmarking Guide](https://gobyexample.com/testing-and-benchmarking)
- [Go GC Guide](https://go.dev/doc/gc-guide)
- [Benchmark Analysis](https://mcclain.sh/posts/go-benchmarking/)