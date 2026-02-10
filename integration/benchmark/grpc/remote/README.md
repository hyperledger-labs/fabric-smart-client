# GRPC benchmarks

This package exercises the grpc server implementation in `platform/view/services/grpc`.

It contains a gRPC client/server benchmark designed to measure throughput (TPS) and tail latency (p99).
The benchmark supports multiple workloads ranging from echo calls to CPU-intensive cryptographic operations (e.g., ECDSA signing), and enables systematic comparison, and deployment modes (local vs remote). 
The benchmark workloads are in the `integration/benchmark/node/remote/workload` package.
Benchmarks are executed with configurable warmup and measurement phases to ensure that results reflect steady-state behavior rather than cold-start effects.

On the client side, load is generated using a configurable number of worker goroutines mapped onto a configurable number of pre-created gRPC connections.
Workers continuously issue RPCs until a deadline is reached, recording per-request latency into histograms after the warmup period.
Benchmark execution is deadline-driven rather than iteration-driven, ensuring consistent measurement intervals across runs.

Note that the TLS enabled by default but the client sets `InsecureSkipVerify=true`.

This is an example output of the benchmark showing the number of executed request, the TPS, and the p5, p95, p99 latencies.

```
benchmarkRemote/w=ecdsa/nc=1-8		   185020	     18502 TPS	    425578 ns/op (p5)	    565395 ns/op (p95)	    629113 ns/op (p99)
benchmarkRemote/w=ecdsa/nc=1-16		   290094	     29009 TPS	    536542 ns/op (p5)	    771338 ns/op (p95)	    878206 ns/op (p99)
```

`plot_grpc.py` is a scripts parse raw benchmark output and generate grouped throughput plots across workloads and connection counts.

## Usage

The server can be started as follows:

```bash
go run ./server/
```

The benchmarks can be run as follows: 

```bash
go run ./client/ -addr=myserver.com:8099 -benchtime=10s -count=5 -workloads=cpu,ecdsa,echo -cpu=1,8,16,32,48,64,96,128,256,376,512,640,768,1024 -numConn=1,2,4,8 2>&1 | tee results.txt
```

