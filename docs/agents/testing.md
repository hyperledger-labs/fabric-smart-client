# Testing

## Frameworks

- **Unit tests**: `github.com/stretchr/testify/require`.
- **Integration tests**: Ginkgo v2 + Gomega.

That is the project convention. Some existing unit suites still use Ginkgo/Gomega and
will be migrated — follow the convention, not the neighbouring file.

## Running tests

Scope every run to what you changed:

```bash
make unit-tests                                        # all unit tests except Postgres
go test -run TestMyTest ./platform/view/...             # one test
TEST_PKGS=./platform/common/utils/... make unit-tests   # one package tree
```

Never run the whole integration suite to check a change — it is slow and needs Fabric
binaries plus Docker. Pick one target, and let CI run the rest:

```bash
make integration-tests-fabric-iou
GINKGO_TEST_OPTS="--focus='IOU Life Cycle'" make integration-tests-fabric-iou
```

[`docs/dev/development.md`](../dev/development.md#running-tests) is the source of truth
for test commands: the remaining targets, the Postgres and coverage variants, and their
prerequisites. See also [integration-tests.md](integration-tests.md).

## Integration test structure

```go
var _ = Describe("Feature", Ordered, func() {
    s := NewTestSuite(opts...)
    BeforeAll(s.Setup)
    AfterAll(s.TearDown)
    It("test case", s.TestFunction)
    It("another test case", s.TestOtherFunction)
})
```

- Define the network topology in `<test>/topology.go` via `integration.Generate()`.
- Put reusable test logic in `<test>/<test>.go`.
- For multi-node scenarios, use `integration.ReplicationOptions`.

### One network per `Describe`

`Setup` generates or loads artifacts and starts every node; `TearDown` stops them. Under
`BeforeEach`/`AfterEach` that cycle runs **per `It`** — roughly 90-110s for a Fabric
topology, against 10-20s for the assertions inside it. A five-spec `Describe` then spends
most of its runtime booting the same network five times.

So default to `Ordered` with `BeforeAll`/`AfterAll` and share one network across the
group. Reach for `BeforeEach` only when a spec genuinely needs a pristine one: it stops a
node, or it writes state a later spec would read. A spec that needs a *different*
topology belongs in its own `Describe` regardless — the topology is fixed when
`NewTestSuite` is called.

Specs that could share a network but would collide on fixed data (the same asset ID, say)
either parameterise that data or stay in separate `Describe`s.

## Focused runs (never commit)

Use `FIt`/`FDescribe` to focus a spec or `XIt` to skip one while iterating —
never commit them (CI treats a stray focus as a failure). Prefer the
`GINKGO_TEST_OPTS="--focus=..."` flag for anything you might commit.

Check that a focused run actually ran something. `go test` prints nothing for a passing
package, so a filter matching **zero specs** reports `ok ... 9.5s`, which reads exactly
like success. Pass `-v` and read `Ran N of M Specs` before believing it.

## Mocks and fakes

Mocks are generated with [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter)
(pinned in `tools/tools.go`). Regenerate with `make generate-mocks` after changing a
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

A file is test-only when nothing outside `_test.go` uses it. Importing `testing` or taking
a `*testing.T/B/TB` is the clearest signal, but not the only one: a helper can build
fixtures (certs, store handles) without touching `testing` at all. Check the callers, not
just the signature.

Do not rely on generic names either way — `platform/fabric/services/state/helpers.go` is
production code despite the name.

When a file mixes production code with a single test helper —
`platform/common/utils/assert/retry.go` (`EventuallyWithRetry`) — it stays in the
report, because excluding it would drop shipped code too. Prefer splitting such a helper
out into a `*_test_utils.go` file, as `platform/common/services/logging` does with
`logging_test_utils.go`.
