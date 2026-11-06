# Testing: Agent Checklist

Read the [Testing Guide](../dev/testing.md) first, and the
[Integration Tests](../dev/integration-tests/README.md) pages for integration work.

- Scope every run to what changed: one package or one test with `go test -run`, one
  integration target. Never run `make integration-tests` to check a change.
- Run one integration target at a time per machine. Other worktrees and other agent
  sessions on the same machine count: targets share Docker container names, ports and
  `out/` directories.
- Before reporting a focused run as passing, run it with `-v` and read `Ran N of M
  Specs`; a filter that matches nothing still prints `ok`.
- A new unit test calls `t.Parallel()`, or carries `//nolint:paralleltest // <state>`
  naming the process-wide state it touches. A package whose tests you made parallel
  passes `go test -race -count=3 -shuffle=on`.
- `require` only on the test's own goroutine; elsewhere `assert`, or send the error back.
- Code that leaves goroutines running after it returns is tested in a `testing/synctest`
  bubble with `synctest.Wait()`, not with `require.Eventually` or sleeps.
- Never commit `FIt`, `FDescribe` or `XIt`.
- Regenerate mocks with `make generate-mocks`; never edit generated mocks or storage
  fixtures by hand.
- A test that exposes a bug is not weakened to pass. Report the failure with its output.
