fix(runner): guard the output cardinality in the serial runner and executor

Type: Task
Template: .github/ISSUE_TEMPLATE/task.yml
Parent: #1861

Labels applied automatically by the template: `status: awaiting triage`
Labels to request from a maintainer: `status: ready for dev`, `skill: good first issue`, `priority: low`

NOTE: paste only the content below the --- line into the issue body. The title
goes in the title field, not the body -- #1869 ended up with a literal
"Title: " prefix in its title.

---

### Description

`pkg/runner/serial.go` indexes the result of its `ExecuteFunc` at `[0]` without
checking the length, in both implementations:

```go
func (r *serialRunner[V]) Run(val V) error {
	return r.executor([]V{val})[0]          // serial.go:20
}

func (r *serialExecutor[I, O]) Execute(input I) (O, error) {
	res := r.executor([]I{input})[0]        // serial.go:35
	return res.Val, res.Err
}
```

An `ExecuteFunc` that returns an empty or `nil` slice panics with
`runtime error: index out of range [0] with length 0`.

The batched sibling validates exactly this contract before pairing outputs to
inputs, at `pkg/runner/batch.go:97`:

```go
if len(inputs) != len(outs) {
	panic(errors.Errorf("expected %d outputs, but got %d", len(inputs), len(outs)))
}
```

So the two implementations of the same interfaces disagree on whether the
one-output-per-input contract is checked. `NewSerialRunner` and
`NewSerialExecutor` are exported and accept a caller-supplied `ExecuteFunc`, so
the contract is only as good as the caller.

The single in-repo caller is safe: `Vault.commitTXs`
(`platform/common/core/generic/vault/vault.go:172`, wired at `vault.go:109`)
returns `collections.Repeat(..., len(txs))` on every path, so it always returns
exactly one output per input. The guard protects downstream implementations of
the exported `ExecuteFunc`, which is why the parent issue files this under
"no in-repo caller today".

Target state: the serial implementations detect a cardinality mismatch and
report it. Both `Run` and `Execute` already return an `error`, so they return one
rather than panicking -- unlike `batch.go`, which runs on a background goroutine
with no error channel back to the caller and therefore has to panic.

### Implementation Steps

- [ ] In `Run`, return an error when the result length is not 1 instead of
      indexing `[0]`
- [ ] In `Execute`, return the zero `O` and an error when the result length is
      not 1
- [ ] Document the one-output-per-input requirement on `ExecuteFunc` in
      `pkg/runner/runner.go`, since it is the contract both implementations rely on
- [ ] Cover empty and over-long results for both implementations in
      `pkg/runner/serial_test.go`
- [ ] Verify: `go test -count=1 -race ./pkg/runner/... ./platform/common/core/generic/vault/...`
      and `make lint`

### Additional Information

Parent: #1861 -- the `pkg/runner/serial.go:20,35` item under "Latent landmines in
exported APIs (no in-repo caller today)".

Removing the guards makes `TestSerialRunner_EmptyResult` and
`TestSerialExecutor_EmptyResult` fail with
`index out of range [0] with length 0`, confirming they cover the fix. The
over-long cases do not panic without the guard -- extra outputs are silently
dropped -- so those two tests pin the contract rather than a crash.

Out of scope: the `panic` at `batch.go:97` and the non-positive-capacity problem
in `batch.go` that #1861 lists as its own item. Changing the batcher's failure
mode from panic to a returned error means giving `batcher.run` a way to signal
its caller, which is a larger change than this one.
