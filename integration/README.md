# Integration

This module holds FSC's integration test suites and `nwo`, the harness that starts the
networks they run on. `integration/fsc/`, `integration/fabric/` and `integration/fabricx/`
hold the suites, `integration/nwo/` the harness, and `integration/benchmark/` the
benchmarks ([Benchmark examples](benchmark/README.md)).

- [Integration Tests](../docs/dev/integration-tests/README.md): every suite, its `make`
  target, what it covers, and how to run it.
- [Authoring Integration Tests](../docs/dev/integration-tests/authoring.md): writing a new
  suite.
- [Fault-Recovery Tests](../docs/dev/integration-tests/fault-recovery.md): injecting faults
  into a running network.
