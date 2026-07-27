# Authoring Integration Tests

Integration tests live in `integration/<platform>/<test-name>/` and use
Ginkgo v2 + Gomega.

## Adding a new test

1. Create the directory `integration/<platform>/<test-name>/`.
2. Define the topology in `topology.go` (via `integration.Generate()`).
3. Implement the test logic in `<test-name>.go`.
4. Add the Ginkgo suite entry point in `<test-name>_test.go`.
5. Register the target in `INTEGRATION_TARGETS` in the `Makefile`.

## Network topology options

```go
opts := &integration.Opts{
    CommType:   fsc.LibP2P, // or fsc.WebSocket
    TLSEnabled: true,
    ReplicationOpts: &integration.ReplicationOptions{
        ReplicationFactors: map[string]int{"node": 3},
        SQLConfigs:         map[string]*postgres.ContainerConfig{ /* ... */ },
    },
}
```

## Test utilities

- `integration.Infrastructure` — network lifecycle management.
- `integration.TestSuite` — base suite with setup/teardown.
- `StartPort()` — dynamic port allocation to avoid collisions.

## Running

```bash
make list-integration-tests
make integration-tests-fabric-iou                       # a single target
make integration-tests                                  # all targets
GINKGO_TEST_OPTS="--focus='IOU Life Cycle'" make integration-tests-fabric-iou
```

Prerequisites (Fabric binaries, Docker images) are covered in
[`docs/dev/development.md`](../dev/development.md).
