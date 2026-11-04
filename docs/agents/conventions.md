# Conventions: Agent Checklist

Read [Code Conventions](../dev/conventions.md) first. Before calling a change finished:

- No `fmt.Errorf` in the changed files: errors come from `pkg/utils/errors`.
- Logging goes through `platform/common/services/logging`.
- Every new exported identifier has a Godoc comment ([Conventions → Godoc](../dev/conventions.md#godoc)).
- A configuration key added, renamed or removed is updated in `docs/configuration.md` and
  in the reference test of the component that reads it, in the same commit; run that
  test (`TestReferenceConfiguration` in `platform/view/services/config`,
  `platform/fabric/core/generic/config` or `platform/fabricx/core/committer/config`).
- A schema change is a new `migrate.Step`; a released step is never edited, reordered or
  removed.
- A proto change only adds fields; run `make buf-breaking`.
- `make checks` passes.
- The documentation is updated in the same commit, in the place
  [Writing Documentation](../dev/documentation.md#where-documentation-goes) names. Never
  put an explanation only in `AGENTS.md` or `docs/agents/`.
