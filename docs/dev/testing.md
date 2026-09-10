# Testing Guide

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
