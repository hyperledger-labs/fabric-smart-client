# Testing Guide

This document outlines how to write and run tests in FSC.

## Unit tests

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

## Fuzzing

Go's native fuzzing mutates test inputs to find panics and other unrecovered crashes; see
the [tutorial](https://go.dev/doc/tutorial/fuzz) and [security
overview](https://go.dev/doc/security/fuzz/) for the mechanics. FSC fuzzes the code that
parses attacker-controlled bytes before signature verification, where a panic is a remotely
triggerable DoS.

The fuzz targets live in
[`platform/fabric/core/generic/transaction/fuzz_test.go`](../../platform/fabric/core/generic/transaction/fuzz_test.go):
`FuzzUnpackSignedProposal`, `FuzzUnpackEnvelopeFromBytes`, `FuzzTransactionSetFromBytes`,
`FuzzTransactionSetFromEnvelopeBytes`. A plain `go test` (via `make unit-tests` and friends)
only replays their in-code seed corpus (the `f.Add(...)` calls) as ordinary subtests; it
does not mutate anything.

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

```
go test fuzz v1
[]byte("...")
```

1. Copy that file into `platform/fabric/core/generic/transaction/testdata/fuzz/<target>/`
   (create the directory if it doesn't exist), keeping its filename.
2. Reproduce it as a regular test: `go test` replays every file under
   `testdata/fuzz/<target>/` whenever `-run` matches, no `-fuzz` flag needed.
   ```bash
   go test -run=<target> ./platform/fabric/core/generic/transaction -v
   ```
   This panics locally with the full stack trace.
3. Fix the root cause. A parser should return an error on malformed input, not panic.
4. Re-run the same command to confirm it now passes.
5. Commit the crasher file alongside the fix. Left in `testdata/fuzz/<target>/`, it becomes
   a permanent regression test that every future `go test` run replays, fuzzing or not.
