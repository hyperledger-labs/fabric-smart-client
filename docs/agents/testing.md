# Testing

## Frameworks

- **Unit tests**: `github.com/stretchr/testify/require`.
- **Integration tests**: Ginkgo v2 + Gomega.

## Running tests

```bash
make unit-tests            # all unit tests except Postgres (-race -cover)
make unit-tests-postgres   # Postgres tests (requires Docker)
make unit-tests-sdk        # SDK wiring tests (TestWiring)

go test -run TestMyTest ./platform/view/...   # a single unit test
```

The full integration suite is slow and needs Fabric binaries + Docker. Run a
focused target locally and let CI run the rest:

```bash
make integration-tests-fabric-iou
GINKGO_TEST_OPTS="--focus='IOU Life Cycle'" make integration-tests-fabric-iou
```

See [integration-tests.md](integration-tests.md) and
[`docs/dev/development.md`](../dev/development.md).

## Integration test structure

```go
var _ = Describe("Feature", func() {
    s := NewTestSuite(opts...)
    BeforeEach(s.Setup)
    AfterEach(s.TearDown)
    It("test case", s.TestFunction)
})
```

- Define the network topology in `<test>/topology.go` via `integration.Generate()`.
- Put reusable test logic in `<test>/<test>.go`.
- For multi-node scenarios, use `integration.ReplicationOptions`.

## Focused runs (never commit)

Use `FIt`/`FDescribe` to focus a spec or `XIt` to skip one while iterating —
never commit them (CI treats a stray focus as a failure). Prefer the
`GINKGO_TEST_OPTS="--focus=..."` flag for anything you might commit.

## Mocks and fakes

Mocks are generated with [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter)
(pinned in `tools/tools.go`). Regenerate with `go generate ./...` after changing a
mocked interface. See [`docs/dev/mocks.md`](../dev/mocks.md).
