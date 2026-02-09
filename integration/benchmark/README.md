# FSC Benchmarks

Performance benchmarks for the Fabric Smart Client framework, measuring throughput (TPS) and latency across different workloads and deployment scenarios.

## Overview

This package contains micro-benchmarks organized into three categories:

### 1. View Workloads (`views/`)
Standalone view implementations defining various workloads:
- **noop**: Minimal no-op view (baseline overhead)
- **cpu**: CPU-intensive workload (200k iterations)
- **ecdsa**: ECDSA P-256 signing operations

Each workload can be benchmarked standalone or used by higher-level benchmarks (gRPC, Node).

### 2. gRPC Benchmarks (`grpc/`)
Benchmarks testing gRPC communication layers:

#### Local Benchmarks
- **`baseline_bench_test.go`**: Raw gRPC baseline (bypasses FSC View Service)
  - Measures pure gRPC overhead without FSC framework
  - Use for comparing FSC implementation overhead vs raw gRPC
- **`grpc_bench_test.go`**: FSC gRPC View Service implementation
  - Tests actual FSC gRPC server/client with mock and real ECDSA signers
  - 6 workload/signer combinations (noop/cpu/ecdsa × mock/ecdsa)

#### Remote Benchmarks (`grpc/remote/`)
- Client/server deployment with histogram-based latency measurement
- Supports configurable workloads, connections, and workers
- Measures TPS and tail latency (p5, p95, p99)
- See `grpc/remote/README.md` for deployment instructions

### 3. Node Benchmarks (`node/`)
Full FSC node benchmarks via View API and gRPC View API:

#### Direct API (`api_bench_test.go`)
- Tests View execution without network overhead
- Two factory variations:
  - **f=0**: Reuses view instance (efficient, measures pure view performance)
  - **f=1**: Creates new view per call (measures view + factory overhead)

#### gRPC API (`api_grpc_bench_test.go`)
- Tests View execution via gRPC with configurable connection counts
- Supports `-numConn` flag for testing 1, 2, 4, 8+ connections
- Always uses f=1 (factory per call) to match realistic usage

#### Remote Benchmarks (`node/remote/`)
- Client/server deployment for realistic network conditions
- Histogram-based latency measurement (p5, p95, p99)
- See `node/remote/README.md` for deployment instructions

## Quick Start

### Run View Benchmarks
```bash
# Run all view benchmarks
go test -bench=. -benchmem -count=10 -cpu=1,2,4,8 ./views/

# Run specific workload
go test -bench=BenchmarkECDSASign -benchmem -count=10 -cpu=1,2,4,8 ./views/
```

### Run gRPC Benchmarks (Local)
```bash
# Run FSC gRPC implementation benchmarks
go test -bench=BenchmarkGRPCImpl -benchmem -count=5 -cpu=1,8,16 ./grpc/

# Run baseline (raw gRPC) for comparison
go test -bench=BenchmarkGRPCBaseline -benchmem -count=5 -cpu=1,8,16 ./grpc/
```

### Run Node Benchmarks (Local)
```bash
# Run direct API benchmarks (f=0 and f=1 variations)
go test -bench=BenchmarkAPI -benchmem -count=5 -cpu=1,8,16 ./node/

# Run gRPC API benchmarks with specific connection counts
go test -bench=BenchmarkAPIGRPC -benchmem -count=5 -cpu=1,8,16 -numConn=1,2,4,8 ./node/
```

### Run Remote Benchmarks

#### gRPC Remote
```bash
# Start server
cd grpc/remote/server && go run main.go

# Run client (in another terminal)
cd grpc/remote/client
go run main.go -addr=localhost:8099 -benchtime=10s -count=5 \
  -workloads=echo,cpu,ecdsa -cpu=1,8,16,32 -numConn=1,2,4
```

#### Node Remote
```bash
# Start server
cd node/remote/server && go run main.go

# Run client (in another terminal)
cd node/remote/client
go run main.go -benchtime=10s -count=5 \
  -workloads=noop,cpu,sign -cpu=1,8,16,32 -numConn=1,2,4
```

## Benchmark Naming Convention

Benchmark names follow this pattern:
```
Benchmark<Component>/<parameters>-<workers>
```

Examples:
- `BenchmarkAPI/w=noop/f=0/nc=0-8`: Direct API, noop workload, view reuse (f=0), 8 workers
- `BenchmarkAPIGRPC/w=cpu/f=1/nc=4-16`: gRPC API, CPU workload, factory per call (f=1), 4 connections, 16 workers
- `BenchmarkGRPCImpl/w=ecdsa/grpcsigner=mock-8`: FSC gRPC, ECDSA workload, mock signer, 8 workers

Parameters:
- **w**: Workload (noop, cpu, ecdsa/sign)
- **f**: Factory mode (0=reuse view, 1=create per call)
- **nc**: Number of connections (0=direct API, 1+=gRPC)
- **grpcsigner**: gRPC signer type (mock, ecdsa)

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
GOGC=100 go test -bench=. -benchmem -count=10 -cpu=1,2,4,8 ./views/ > plots/benchmark_gc_100.txt
GOGC=off go test -bench=. -benchmem -count=10 -cpu=1,2,4,8 ./views/ > plots/benchmark_gc_off.txt
```

## Visualization

The `plots/` directory contains Python scripts to visualize benchmark results:

```bash
# Setup
cd plots
python3 -m venv env
source env/bin/activate
pip3 install -r requirements.txt

# Generate plots for different benchmark types
python3 plot_view.py benchmark_gc_100.txt benchmark_gc_100.pdf
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
- **p5/p95/p99**: Latency percentiles (for remote benchmarks)
- **B/op**: Bytes allocated per operation
- **allocs/op**: Allocations per operation

Example output:
```
BenchmarkAPIGRPC/w=ecdsa/f=1/nc=2-16    236702    23670 TPS    654140 ns/op (p5)    847374 ns/op (p95)
```

## Benchmark Comparison Guide

### Baseline vs Implementation
Compare `BenchmarkGRPCBaseline` vs `BenchmarkGRPCImpl` to measure FSC framework overhead:
```bash
go test -bench='Benchmark(GRPCBaseline|GRPCImpl)' -benchmem -count=10 ./grpc/
```

### Factory Overhead
Compare f=0 vs f=1 in `BenchmarkAPI` to measure view factory overhead:
```bash
go test -bench='BenchmarkAPI/w=ecdsa' -benchmem -count=10 ./node/
```

### Connection Scaling
Test how performance scales with connection count:
```bash
go test -bench=BenchmarkAPIGRPC -benchmem -count=5 -cpu=16 -numConn=1,2,4,8,16 ./node/
```

### Mock vs Real Crypto
Compare mock vs ECDSA signers in gRPC benchmarks:
```bash
go test -bench='BenchmarkGRPCImpl/w=noop' -benchmem -count=10 ./grpc/
```

## Best Practices

1. **Warmup**: Benchmarks include warmup phases to reach steady-state
2. **Multiple Runs**: Use `-count=5` or higher for statistical significance
3. **CPU Scaling**: Test with `-cpu=1,2,4,8,16` to measure scalability
4. **GC Impact**: Compare results with different GOGC settings
5. **Isolation**: Run on idle systems for consistent results
6. **Baseline Comparison**: Use baseline benchmarks to measure framework overhead
7. **Remote Testing**: Use `node/remote/` or `grpc/remote/` for realistic network conditions
8. **Factory Testing**: Use f=0 vs f=1 to understand factory overhead impact

## Troubleshooting

### High Variance in Results
- Ensure system is idle (no other processes consuming CPU)
- Increase `-count` for more samples
- Check for thermal throttling on laptops
- Run with `GOGC=off` to eliminate GC variance

### Low TPS
- Check if `-cpu` matches available cores
- Verify workload isn't I/O bound
- Compare against baseline to identify bottlenecks
- Profile with `go test -cpuprofile=cpu.prof`

### Remote Benchmarks Failing
- Verify server is running and accessible
- Check firewall rules for port 8099
- Ensure client config points to correct server address
- Review server logs for errors

## Resources

- [Go Benchmarking Guide](https://gobyexample.com/testing-and-benchmarking)
- [Go GC Guide](https://go.dev/doc/gc-guide)
- [Benchmark Analysis](https://mcclain.sh/posts/go-benchmarking/)
- [FSC Documentation](../../docs/)
