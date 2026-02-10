# FSC Node GRPC View API benchmarks

This package exercises the GRPC View API implementation `platform/view/services/view/grpc`.

It contains a gRPC client/server benchmark designed to measure throughput (TPS) and tail latency (p99).
The benchmark supports multiple workloads views ranging from no-op calls to CPU-intensive cryptographic operations (e.g., ECDSA signing), and enables systematic comparison, and deployment modes (local vs remote). 
The benchmark workloads views are defined in `integration/benchmark/views` package.
Benchmarks are executed with configurable warmup and measurement phases to ensure that results reflect steady-state behavior rather than cold-start effects.

On the client side, load is generated using a configurable number of worker goroutines mapped onto a configurable number of pre-created gRPC connections.
Workers continuously issue RPCs until a deadline is reached, recording per-request latency into histograms after the warmup period.
Benchmark execution is deadline-driven rather than iteration-driven, ensuring consistent measurement intervals across runs.

Note that the TLS enabled by default but the client sets `InsecureSkipVerify=true`.

This is an example output of the benchmark showing the number of executed request, the TPS, and the p5, p95, p99 latencies.

```
BenchmarkAPIGRPCRemote/w=noop/nc=2-8		   130048	     13005 TPS	    597368 ns/op (p5)	    753319 ns/op (p95)	    863278 ns/op (p99)
BenchmarkAPIGRPCRemote/w=noop/nc=2-16		   236702	     23670 TPS	    654140 ns/op (p5)	    847374 ns/op (p95)	   1396724 ns/op (p99)
```

`plot_node.py` is a script that parses raw benchmark output and generate grouped throughput plots across workloads and connection counts.

## Usage

The server can be started as follows:

```bash
go run ./server/main.go
```

The server generates the crypto material folder in `out/testdata`.


The benchmarks can be run as follows: 

```bash
# get the testdata folder from the server and place it next to our benchmark client
rsync -av --exclude='.git' --exclude='env/' user@myserver.com:/home/user/fabric-smart-client/integration/benchmark/node/remote /home/user/fabric-smart-client/integration/benchmark/node/remote

# update the connection file and change server address from 127.0.0.1:8099 to your server address, for example, `myserver.com:8099` 
vim /home/user/fabric-smart-client/integration/benchmark/node/remote/out/testdata/fsc/nodes/test-node.0/client-config.yaml

go run ./client/main.go -benchtime=10s -count=5 -workloads=noop,cpu,sign -cpu=1,8,16,32,48,64,96,128,256,376,512,640,768,1024 -numConn=1,2,4,8 2>&1 | tee results.txt
```

