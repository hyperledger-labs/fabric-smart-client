# Testing guide

This document outlines how to write and run tests in FSC. New functionality comes with
tests in the same PR; the policy, and the few changes it exempts, is in
[Contribution Workflow → Tests](workflow.md#tests). Integration tests have a section of
their own: [Integration Tests](integration-tests/README.md).

## Frameworks

- **Unit tests**: testify, `require` by default and `assert` where [Choosing `assert` over `require`](#choosing-assert-over-require) says so.
- **Integration tests**: Ginkgo v2 + Gomega.
- **`nwo` unit tests** in the integration module: Gomega, because `nwo` asserts through it.

A `depguard` rule in [`.golangci.yml`](../../.golangci.yml) rejects Gomega and Ginkgo
imports in test files outside `integration/`.

## Running tests

Scope every run to what you changed:

```bash
make unit-tests                                             # all unit tests except root-module Postgres tests
make unit-tests-root                                        # only the root module
go test -run TestMyTest ./platform/view/...                  # one test
TEST_PKGS=./platform/common/utils/... make unit-tests-root   # one package tree
```

Never run the whole integration suite to check a change — it is slow and needs Fabric
binaries plus Docker. Pick one target, and let CI run the rest:

```bash
make integration-tests-fabric-iou
GINKGO_TEST_OPTS="--focus='IOU Life Cycle'" make integration-tests-fabric-iou
```

The [Development Guide](development.md#running-tests) is the source of truth
for test commands: the remaining targets, the Postgres and coverage variants, and their
prerequisites.

## Unit tests

### Parallel unit tests

A unit test and each of its subtests call `t.Parallel()`, so `go test -race` exercises
them concurrently and `-shuffle=on` and `-count` vary their order. A test that reads or
changes process-wide state (`t.Setenv`, `logging.Init`, the global Gomega, a package
variable such as `prom.DefaultRegisterer`) runs serially instead and says which state
with `//nolint:paralleltest // <state>`. Placed on the line above the `func`, the
directive also covers that test's subtests.

### Goroutines that outlive the call

When the code under test leaves goroutines running after it returns, as the BFT
broadcaster does with the sends to slower orderers, run the test in a
[`testing/synctest`](https://pkg.go.dev/testing/synctest) bubble:
`synctest.Test(t, func(t *testing.T) { ... })`. `synctest.Wait()` returns once every
other goroutine in the bubble is blocked or done, so the test asserts on a settled state
instead of polling with `require.Eventually`, and timers run on the bubble's fake clock,
so a 10-second timeout passes at once. `synctest.Test` fails the test if a goroutine is
still blocked when its function returns: release blocked fakes with `t.Cleanup`.
`platform/fabric/core/generic/ordering/bft_test.go` has examples.

### Choosing `assert` over `require`

`require` stops the test immediately on failure (`t.FailNow()`, via `runtime.Goexit()`);
`assert` records the failure and lets the test keep running. At each call site, ask
whether continuing past this failure would produce a misleading result or a crash, or
whether it's an independent check.

**Use `require` when:**

- A later line dereferences, indexes, or type-asserts the value just checked, e.g.
  `require.NoError(t, err)` before `client.Do()`, or `require.NotNil(t, x)` before
  `x.Field`. Without it, a nil value or an error here panics instead of failing the
  test cleanly, and the panic message says far less than the assertion would have.
- The checks form a sequence where each step depends on the last one succeeding (a
  Put → Get → Delete → Get flow, setup before the real test body). One failure here
  makes every assertion after it noise.
- It's a manual `if err != nil { t.Fatalf(...) }` — that's `require.NoError` spelled
  out by hand, so convert it.

**Use `assert` when:**

- The checks are independent, such as a table-driven test verifying several unrelated
  fields, where you want to see every failure in one run rather than stopping at the
  first.
- The call runs off the test's main goroutine: inside a spawned `go func()`, `wg.Go()`,
  or an `httptest` handler closure. `Goexit()` is unsafe to call from any goroutine but
  the test's own, so `require` there can hang the test or corrupt the result instead of
  failing it cleanly.
- The call sits inside an `EventuallyWithT` callback. Both `assert.EventuallyWithT` and
  `require.EventuallyWithT` take the same callback signature,
  `func(collect *assert.CollectT)` — testify's `require` package defines no `CollectT`
  of its own — so calls inside that callback are `assert.X` regardless of which outer
  function you used.

Default to `require`, and drop to `assert` only for one of the three reasons above.
This is a per-call-site judgment, not a mechanical swap.

### Mocks and fakes

Mocks are generated with [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter)
(pinned in `tools/tools.go`). Regenerate with `make generate-mocks` after changing a
mocked interface. See [Mocks](mocks.md).

### Shared test helpers

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

### Storage fixtures

`platform/fabric/services/db/driver/sql/testdata/<release>/` holds the SQLite file and the
Postgres layout and rows that each release wrote, with checksums. `TestFixturesSQLite` and
`TestPostgresFixtures` open them at HEAD, migrate every store and read the rows back. A
release adds its fixture directory in its release PR with
`testdata/generate.sh --local <release>`, and the tests pick it up; see
[the fixture README](../../platform/fabric/services/db/driver/sql/testdata/README.md) and
[Releasing](releasing.md). Never edit a fixture by hand. The `Storage fixtures` workflow
also generates the latest release's fixture into a temporary directory and opens it
through `FSC_FIXTURE_DIR`, so a change to `generate.go` must keep building against that
release.

## Fuzzing

Go's native fuzzing mutates test inputs to find panics and other unrecovered crashes; see
the [tutorial](https://go.dev/doc/tutorial/fuzz) and [security
overview](https://go.dev/doc/security/fuzz/) for the mechanics. FSC fuzzes the code that
parses attacker-controlled bytes before signature verification, where a panic is a remotely
triggerable DoS.

The fuzz targets live across several packages handling untrusted pre-auth bytes:

- [`platform/fabric/core/generic/transaction/fuzz_test.go`](../../platform/fabric/core/generic/transaction/fuzz_test.go):
  `FuzzUnpackSignedProposal`, `FuzzUnpackEnvelopeFromBytes`, `FuzzTransactionSetFromBytes`,
  `FuzzTransactionSetFromEnvelopeBytes`, `FuzzNewProposalResponseFromBytes`.
- [`platform/fabricx/core/transaction/fuzz_test.go`](../../platform/fabricx/core/transaction/fuzz_test.go):
  `FuzzFabricxNewProposalResponseFromBytes`, `FuzzFabricxUnpackEnvelopeFromBytes`.
- [`platform/fabric/protos/fuzz_test.go`](../../platform/fabric/protos/fuzz_test.go):
  `FuzzJSONFromEnvelope`, a received transaction envelope converted to JSON.
- [`platform/fabric/core/generic/msp/x509/fuzz_test.go`](../../platform/fabric/core/generic/msp/x509/fuzz_test.go):
  `FuzzDeserializeVerifier`.
- [`platform/view/services/comm/io/fuzz_test.go`](../../platform/view/services/comm/io/fuzz_test.go):
  `FuzzVarintReaderReadData`.
- [`platform/view/services/comm/fuzz_test.go`](../../platform/view/services/comm/fuzz_test.go):
  `FuzzPKExtractorExtractPublicKey`.
- [`platform/view/services/comm/host/websocket/ws/fuzz_test.go`](../../platform/view/services/comm/host/websocket/ws/fuzz_test.go):
  `FuzzMultiplexedMessage`, one WebSocket message handled by a server connection's read loop.

A plain `go test` (via `make unit-tests` and friends) only replays their in-code seed corpus
(the `f.Add(...)` calls) as ordinary subtests; it does not mutate anything.

Actual mutation-based fuzzing runs in
[`.github/workflows/fuzz.yml`](../../.github/workflows/fuzz.yml):

- Nightly, one job per target, 2h `-fuzztime` each, also runnable on demand via
  `workflow_dispatch`.
- The corpus each run discovers is cached between runs (keyed per target), so fuzzing
  resumes instead of restarting cold every night.
- A target that finds a crash fails its job and uploads the crashing input as a build
  artifact named `fuzz-crash-<target>`.

To fuzz a target locally instead of waiting for CI:

```bash
go test ./platform/fabric/core/generic/transaction \
  -run='^$' -fuzz='^FuzzUnpackSignedProposal$' -fuzztime=1m
```

### Reproducing a crash from a CI artifact

When a nightly run fails, download its `fuzz-crash-<target>` artifact from the workflow
run's summary page and unzip it. It contains one file, named by content hash, holding the
exact input that crashed the target:

```text
go test fuzz v1
[]byte("...")
```

1. Copy that file into `<target-package-path>/testdata/fuzz/<target>/`
   (e.g., `platform/fabric/core/generic/msp/x509/testdata/fuzz/<target>/`),
   keeping its filename (create the directory if it doesn't exist).
2. Reproduce it as a regular test: `go test` replays every file under
   `testdata/fuzz/<target>/` whenever `-run` matches, no `-fuzz` flag needed.

   ```bash
   go test -run=<target> ./<target-package-path> -v
   ```

   This panics locally with the full stack trace.
3. Fix the root cause. A parser should return an error on malformed input, not panic.
4. Re-run the same command to confirm it now passes.
5. Commit the crasher file alongside the fix. Left in `testdata/fuzz/<target>/`, it becomes
   a permanent regression test that every future `go test` run replays, fuzzing or not.
