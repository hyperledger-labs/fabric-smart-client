# Integration Tests: Agent Checklist

Read [Integration Tests](../dev/integration-tests/README.md),
[Authoring Integration Tests](../dev/integration-tests/authoring.md) and, for specs that
inject faults, [Fault-Recovery Tests](../dev/integration-tests/fault-recovery.md) first.

- A new suite has `topology.go`, a `_suite_test.go` with
  `var _ = AfterSuite(integration.CleanupBuildArtifacts)`, and a target in the `Makefile`.
- Label every `Describe` with the comm type it runs, and name every `It` after the
  behaviour it checks.
- Code two suites use goes to `integration/fabric/common/` or `integration/fabricx/common/`;
  a suite never imports another suite's package.
- Keep `integration`, `integration/nwo/...` and `integration/benchmark/...` compatible:
  Panurus imports them.
- `integration/fabric/mixedversion/views` runs at the previous release too: change it only
  compatibly, using API that release has; see
  [Nodes of another release](../dev/integration-tests/authoring.md#nodes-of-another-release).
- A fault scenario that applies to both platforms goes into a shared spec group in
  `integration/fabric/common/faultrecovery`, so both suites run it. A spec under
  `ShareNetwork` leaves the network healthy and leaks no sessions or view contexts.
- No `time.Sleep` where a signal exists; derive bounds from the timeouts the code defines
  (`SessionCloseTimeout`, `RecoveryTimeout`).
- A change that renames, moves or relabels specs shows with `ginkgo --dry-run -v`, using each
  affected target's label filter, that every spec still runs in exactly one target.
- A change to CI time puts the affected job times before and after in the PR.
- A new helper, spec group, target or label is added to the tables in the pages for
  people in the same commit.
