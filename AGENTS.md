# AGENTS.md

**Fabric Smart Client (FSC)** is a client-side framework for Hyperledger Fabric
and Fabric-x that models distributed business processes as interactive protocols
of composable *views*, so developers write business logic instead of low-level
blockchain plumbing.

Go project (root module `github.com/hyperledger-labs/fabric-smart-client`),
built with `make`. `CLAUDE.md` is a symlink to this file.

## Essential commands

```bash
make checks         # golangci-lint + `go fix` in every module; apply fixes with `make go-fix-apply`
make lint           # golangci-lint (use make lint-auto-fix to autofix)
make tidy           # go mod tidy across all modules
make generate-protos     # regenerate protobuf files
make install-tools       # install dev tools (source of truth: tools/tools.go)
make unit-tests     # unit tests in every module, excluding Postgres (-race -cover)
make unit-tests-root     # one module only: -root, -integration or -extensions
make integration-tests   # integration tests (need Fabric binaries + Docker)
```

If `make lint` reports a file outside your working tree, the `golangci-lint` cache is
stale — common with several worktrees. Run `golangci-lint cache clean` and re-run; see
[`docs/dev/development.md`](docs/dev/development.md#lint-failures-in-files-you-never-touched).

The `make` defaults assume the **primary checkout**: `FABRIC_BINARY_BASE` is
`$(PWD)/../fabric`, so `make install-fabric-bins` run from a worktree installs a second
Fabric next to that worktree. Pass `FABRIC_BINARY_BASE` explicitly, or run it from the
primary checkout — see [`docs/dev/development.md`](docs/dev/development.md#fabric).

Run one unit test: `go test -run TestMyTest ./platform/view/...`. The full
integration suite is slow and needs Fabric binaries + Docker — run a focused
target locally (`make integration-tests-fabric-iou`) and let CI run the rest.
First-time setup, Fabric binaries, and Docker images are in
[`docs/dev/development.md`](docs/dev/development.md).

## Modules

FSC is a multi-module repository. `go build ./...` / `go test ./...` from the
root only sees the root module — the other four are invisible to it.

| Module | Path | Notes |
|--------|------|-------|
| root | `.` | the framework itself |
| integration | `integration/` | test harness (`nwo`) + integration suites; `replace`s root, `cc/query`, `libp2p` |
| libp2p host | `platform/view/services/comm/host/libp2p/` | optional comm driver; `replace`s root |
| chaincode query | `platform/fabric/services/state/cc/query/` | chaincode, does not depend on root |
| tools | `tools/` | dev-tool pins (`module tools`); not released |

- **Dependency changes**: run `make tidy` (`scripts/gomate.sh tidy` — tidies *every*
  module), not `go mod tidy` in one place. To bump a dep everywhere:
  `./scripts/gomate.sh update github.com/some/dep@v1.2.3`.
- A dependency used only by code under `integration/` or the libp2p host
  belongs in *that* module's `go.mod`, not the root one.
- **Releases** tag each module separately
  (`make tag-release VERSION=vX.Y.Z`, see [`scripts/tag-release.sh`](scripts/tag-release.sh));
  a change in a submodule needs its own tag to be consumable downstream.

## Where to look next

Read these on demand — don't load them up front.

| Topic | Doc |
|-------|-----|
| View/session programming model (views, sessions, initiator/responder) | [`docs/platform/view/programming-model.md`](docs/platform/view/programming-model.md) |
| Platform layout, SDK composition, `dig` DI, multi-network | [`docs/agents/architecture.md`](docs/agents/architecture.md) |
| Code organization, errors, logging, storage, identity, security | [`docs/agents/conventions.md`](docs/agents/conventions.md) |
| Unit + integration test conventions | [`docs/agents/testing.md`](docs/agents/testing.md) |
| Testing best practices (e.g. `assert` vs `require`) | [`docs/dev/testing.md`](docs/dev/testing.md) |
| Authoring a new integration test | [`docs/agents/integration-tests.md`](docs/agents/integration-tests.md) |
| Node configuration (`core.yaml`) | [`docs/configuration.md`](docs/configuration.md) |
| Architecture overview & concepts | [`docs/core-concepts.md`](docs/core-concepts.md) |
| Contribution workflow | [`docs/dev/workflow.md`](docs/dev/workflow.md), [`CONTRIBUTING.md`](CONTRIBUTING.md) |

## Documentation

Documentation — Godoc comments and standalone docs alike — describes the
**current implementation as a self-contained system**. Write for a developer who
cloned the repository today: never saw the previous implementation, does not know
the git history, has not read the PR or issue. If a doc only makes sense to
someone who does, rewrite it.

- **Not a changelog.** Never write "previously", "before this change", "we
  changed/moved from X to Y", "formerly", "now instead of", "this replaces",
  "the old implementation", "after the refactor". Don't explain a design by
  describing what the code used to do.
- **Explain WHY the current design exists**, in terms of today's requirements —
  correctness, concurrency, ordering, lifecycle, security, performance, resource
  ownership, API guarantees, compatibility, failure handling — and only when that
  rationale helps understand or maintain the code. Prefer *"The client uses X to
  coordinate concurrent requests and preserve ordering."* over *"We previously
  used Y, but changed it to X."*
- **History is an input, not a subject.** Inspect commits, PRs, and issues freely
  while investigating why the code looks the way it does; document the resulting
  design, not the investigation. Linking an issue for extra context is fine as
  long as the doc still explains the behavior on its own.
- **Godoc** covers what an exported package/type/function represents, its
  responsibilities, guarantees, preconditions, concurrency and lifecycle
  requirements, error behavior, and semantics not obvious from the signature —
  never why the code changed, what the old version did, or which PR introduced
  it. Comments must stay useful with no git history available. Details:
  [`docs/agents/conventions.md`](docs/agents/conventions.md#godoc).
- **Write for maintainers**: responsibilities and ownership, component
  relationships, important execution flows, invariants, lifecycle, concurrency
  model, error handling, resource management, extension points, and assumptions
  that must remain true. Don't narrate obvious code.
- **Keep docs aligned with the code** in the same commit: update Godoc when
  behavior or semantics change, update standalone docs when architecture or
  externally visible behavior changes, and delete documentation for behavior that
  no longer exists. Historical wording is not preserved just because it was once
  accurate.
- **Style**: precise, concise, technical, factual, present tense; no narrative.

## Conventions in one line

- **Errors**: use `pkg/utils/errors` (`errors.New/Errorf/Wrap/Wrapf/WithMessage/WithMessagef/Join`); do not build or wrap errors with `fmt.Errorf`.
- **Logging**: `platform/common/services/logging`.
- **New code**: platform code → `platform/<name>/`; shared → `pkg/utils/` or `platform/common/`.
- **DI**: register in the platform's `sdk/dig/sdk.go` (e.g. `platform/view/sdk/dig/sdk.go`); `Install()` must call the parent `p.SDK.Install()`.
- **Mocks**: `counterfeiter` via `make generate-mocks` (see [`docs/dev/mocks.md`](docs/dev/mocks.md)).
- **Git**: run `make checks` before committing; sign off every commit (`git commit -s`, DCO); rebase, don't merge. Fixups during review, then autosquash: a PR merges as one commit whose message describes the PR. See [`docs/dev/workflow.md`](docs/dev/workflow.md#commit-hygiene), [`docs/dev/signing.md`](docs/dev/signing.md), [`docs/dev/rebasing.md`](docs/dev/rebasing.md).
- **Docs**: a change that leaves [`docs/agents/`](docs/agents/) or [`docs/dev/`](docs/dev/) wrong is not finished — update the affected guide in the same commit; see [Documentation](#documentation).

## Related projects

- **Panurus** (token layer built *on* FSC, formerly Token SDK): <https://github.com/LFDT-Panurus/panurus>
- **Fabric-x Committer** (what the `fabricx` platform submits to): <https://github.com/hyperledger/fabric-x-committer>
- Community: LFDT Discord `#fabric-smart-client`
