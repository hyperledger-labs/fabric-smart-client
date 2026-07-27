# AGENTS.md

**Fabric Smart Client (FSC)** is a client-side framework for Hyperledger Fabric
and Fabric-x that models distributed business processes as interactive protocols
of composable *views*, so developers write business logic instead of low-level
blockchain plumbing.

Go project (root module `github.com/hyperledger-labs/fabric-smart-client`),
built with `make`. `CLAUDE.md` is a symlink to this file.

## Essential commands

```bash
make checks         # license, gofmt, goimports, vet, misspell, staticcheck
make lint           # golangci-lint (use make lint-auto-fix to autofix)
make tidy           # go mod tidy across all modules
make generate-protos     # regenerate protobuf files
make install-tools       # install dev tools (source of truth: tools/tools.go)
make unit-tests     # unit tests, excluding Postgres (-race -cover)
make integration-tests   # integration tests (need Fabric binaries + Docker)
```

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
| Authoring a new integration test | [`docs/agents/integration-tests.md`](docs/agents/integration-tests.md) |
| Node configuration (`core.yaml`) | [`docs/configuration.md`](docs/configuration.md) |
| Architecture overview & concepts | [`docs/core-concepts.md`](docs/core-concepts.md) |
| Contribution workflow | [`docs/dev/workflow.md`](docs/dev/workflow.md), [`CONTRIBUTING.md`](CONTRIBUTING.md) |

## Conventions in one line

- **Errors**: use `pkg/utils/errors` (`errors.New/Errorf/Wrap/Wrapf/WithMessage/WithMessagef/Join`); do not build or wrap errors with `fmt.Errorf`.
- **Logging**: `platform/common/services/logging`.
- **New code**: platform code → `platform/<name>/`; shared → `pkg/utils/` or `platform/common/`.
- **DI**: register in `sdk/dig/sdk.go`; `Install()` must call the parent `p.SDK.Install()`.
- **Mocks**: `counterfeiter` via `go generate ./...` (see [`docs/dev/mocks.md`](docs/dev/mocks.md)).
- **Git**: sign off every commit (`git commit -s`, DCO); rebase, don't merge. See [`docs/dev/signing.md`](docs/dev/signing.md), [`docs/dev/rebasing.md`](docs/dev/rebasing.md).

## Related projects

- **Panurus** (token layer built *on* FSC, formerly Token SDK): <https://github.com/LFDT-Panurus/panurus>
- **Fabric-x Committer** (what the `fabricx` platform submits to): <https://github.com/hyperledger/fabric-x-committer>
- Community: LFDT Discord `#fabric-smart-client`
