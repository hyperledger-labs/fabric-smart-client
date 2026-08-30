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

## Shared test helpers

Helpers that other packages import cannot live in a `_test.go` file, so conformance
suites, benchmark bodies, and node harnesses sit in plain `.go` files — where they
would otherwise be counted as production code.

**Name them `*_test_utils.go`.** That single convention is the whole mechanism:
`scripts/filter-coverage.sh` matches the pattern and keeps them out of the coverage
denominator, exactly as it already excludes mocks and fakes. Nothing enforces the
name, and a misnamed helper is silently counted as production code — so apply the
convention when you add one.

A file counts as test-only when it imports `testing` and *every* exported function
takes a `*testing.T/B/TB`. Do not rely on generic names for this —
`platform/fabric/services/state/helpers.go` and
`platform/fabric/services/storage/vault/helpers.go` are production code.

When a file mixes production code with a single test helper —
`platform/common/services/logging/logger.go` (`NewTestLogger`),
`platform/common/utils/assert/retry.go` (`EventuallyWithRetry`) — it stays in the
report, because excluding it would drop shipped code too. Prefer splitting such a
helper into a `*_test_utils.go` file over leaving it mixed.
