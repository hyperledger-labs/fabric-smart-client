# Comm Benchmarks

Micro-benchmarks of the P2P communication layer (`platform/view/services/comm`). Each benchmark
runs once per transport, `websocket` and `libp2p`, between nodes on loopback, so the two
transports can be compared under identical workloads.

The nodes are real `comm.P2PNode`s over real hosts: websocket uses mutual TLS with
certificates issued by a throwaway CA and a static ID router, and libp2p uses a bootstrap node
and nodes that join through it. Keys are generated per benchmark; no fixtures or Fabric
binaries are needed.

## Benchmarks

| Benchmark | Measures |
|---|---|
| `BenchmarkRoundTrip/transport=<t>/size=<n>` | Steady-state latency of one request/echo exchange on a single long-lived session, for 64 B, 1 KiB, 64 KiB and 1 MiB payloads. Session setup and the first dial are excluded. |
| `BenchmarkSessionSetup/transport=<t>` | One full session lifecycle per op: open, one 64 B exchange, close on both sides. The transport connection is already up. |
| `BenchmarkParallelRoundTrip/transport=<t>` | Throughput of `GOMAXPROCS` concurrent senders, each on its own session, sharing the node pair. 1 KiB payloads. The sessions are opened before timing starts. |
| `BenchmarkMesh/transport=<t>/nodes=<n>/sessions=<s>` | A full mesh of 3 or 5 nodes with 1 or 4 long-lived sessions per ordered pair, all exchanging 1 KiB round trips at once. |

Units:
- `ns/op` is the mean time per op. In the parallel and mesh benchmarks ops overlap, so it is wall time divided by ops, the inverse of throughput.
- `p50-ns` and `p99-ns` are the median and 99th percentile of a single op, measured by the sender: one round trip, or in `BenchmarkSessionSetup` one full session lifecycle.
- `TPS` is ops per second. `MB/s` counts payload bytes in both directions.
- `B/op` and `allocs/op` cover both ends of the exchange, since all nodes run in the benchmark process.

## Running

```bash
cd integration
go test -run '^$' -bench . -benchtime 2s -count 10 ./benchmark/comm/ | tee comm.txt
benchstat -col /transport comm.txt   # websocket vs libp2p side by side
```

Logs are off by default so that they do not interleave with results; set `FSC_LOGSPEC` (e.g. `error` or `fsc.platform.view.services.comm=debug:error`) to see them.
