# Development Guide

This document provides guidelines and steps for setting up your local development environment for the **Fabric Smart Client (FSC)**.

For general development best practices, see the following guidelines:  
- [Fabric-x Committer Development Guidelines](https://github.com/hyperledger/fabric-x-committer/blob/main/guidelines.md)
- [Fabric TokenSDK Development Guidelines](https://github.com/hyperledger-labs/fabric-token-sdk/blob/main/docs/development/development.md)

## Prerequisites

Before you begin, ensure you have the following installed:

- **Go** — [Install Go](https://go.dev/doc/install) (see the required version in [`go.mod`](../../go.mod))
- **Docker** — [Install Docker Engine](https://docs.docker.com/engine) (or a compatible container manager)

## Clone the Repository

Clone the FSC repository to your local workspace.  
Throughout this document, `$FSC_PATH` refers to the local path of your cloned repository.

```bash
export FSC_PATH=$HOME/myprojects/fabric-smart-client
git clone https://github.com/hyperledger-labs/fabric-smart-client.git $FSC_PATH
cd $FSC_PATH
```

## Setting Up Developer Tools

FSC provides several helper tools for building, testing, and monitoring.
Install them using:

```bash
make install-tools install-linter-tool pull-images-monitoring pull-images-database
```

Platform-specific tools are also required for **Fabric** and **Fabric-x**.

### Fabric

Install the Fabric binaries and Docker images:

```bash
make install-fabric-bins pull-images-fabric
```

> [!IMPORTANT]
> `FABRIC_BINARY_BASE` defaults to `$(PWD)/../fabric`, which assumes you are in your
> primary checkout. Run this from a worktree under `.claude/worktrees/<name>/` and it
> resolves to a sibling of the *worktree* — a second Fabric install, in the wrong
> place, inside the repository, with no warning. From a worktree, either run the target
> from the primary checkout or pass the base explicitly:
>
> ```bash
> FABRIC_BINARY_BASE=$HOME/fabric make install-fabric-bins
> ```

Integration tests that deploy chaincode as a container also need the chaincode
images built once:

```bash
make chaincode-images
```

Rebuild them whenever you change a chaincode under `integration/fabric/*/chaincode*`
or `integration/nwo/fabric/chaincode/base`.

To install a specific Fabric version, set the `FABRIC_VERSION` variable:

```bash
FABRIC_VERSION=3.1.0 make install-fabric-bins
```

The default `FABRIC_VERSION` is defined in the project [Makefile](../../Makefile). 


### Fabric-x

Install Fabric-x configuration tools and Docker images:

```bash
make install-fabricx-tools pull-images-fabricx
```

Fabric-x ships a `configtxgen` and a `cryptogen` of its own, so the tools are
installed into a `fabric-x` subdirectory of `FAB_BINS` rather than next to
fabric's binaries of the same name. Both toolchains can be installed at the same
time, and neither install target disturbs the other.

### Set `FAB_BINS`

Most integration tests require Fabric(x) binaries to launch a local test network.
Set the `FAB_BINS` environment variable to point to the directory containing these binaries:

```bash
export FAB_BINS=/home/yourusername/fabric/bin
```

One variable covers both platforms: `fabric-*` suites read `$FAB_BINS`, and
`fabricx-*` suites read `$FAB_BINS/fabric-x`, which is where
`make install-fabricx-tools` puts its tools.

> [!NOTE]
> Do *not* store the Fabric binaries inside your fabric-smart-client repository.
Doing so may cause integration tests to fail when installing chaincode.

## Running Tests

FSC includes both unit tests and integration tests.
Integration tests are powered by the NWO (Network Orchestrator), which programmatically creates DLT networks and FSC application nodes.

### Code Checks

`make checks` runs everything CI gates on — the linter and `go fix` — across every
module in the repository:

```bash
make checks
```

| Target               | What it does                                                          |
|----------------------|-----------------------------------------------------------------------|
| `make lint`          | `golangci-lint run` in every module                                   |
| `make lint-auto-fix` | the same, with `--fix` to apply what `golangci-lint` can fix itself   |
| `make lint-fmt`      | `golangci-lint fmt`, applying the formatter configuration CI enforces  |
| `make fmt`           | `gofmt -s -w` over the whole tree                                     |
| `make go-fix`        | reports the modernizations `go fix` suggests, without applying them    |
| `make go-fix-apply`  | applies those modernizations, in every module                          |

Run `make list-go-modules` to see which modules these targets cover.

> [!NOTE]
> Checking formatting by path — `gofmt -l platform integration` — also walks gitignored
> generated files, such as everything an integration run leaves under
> `integration/**/out/`, so your source looks unformatted when it is not. Check the
> tracked files instead:
>
> ```bash
> gofmt -l $(git ls-files '*.go')
> ```

### Unit Tests

Run all unit tests:
```bash
make unit-tests
make unit-tests-postgres
make unit-tests-sdk
```

Use `make unit-tests` for the default unit-test suite.
Use `make unit-tests-postgres` only for tests that explicitly exercise the PostgreSQL-backed storage implementations.
Use `make unit-tests-sdk` when validating dependency-injection wiring in SDK packages.

When your change is confined to one module, run just that module's target:

```bash
make unit-tests-root           # the framework itself
make unit-tests-integration    # the nwo harness and the helpers the suites share
make unit-tests-extensions     # optional driver modules (today: the libp2p comm host)
```

To keep the feedback loop fast while working on a single package, scope the root-module
run with `TEST_PKGS`:

```bash
TEST_PKGS=./platform/common/utils/... make unit-tests-root
TEST_PKGS=./platform/common/utils/dig go test -race -cover ./platform/common/utils/dig
```

For coverage analysis, `COVERAGE=1` makes any unit-test target leave a filtered
profile (the same filter CI reports through) in `./coverage.profile`; override the
path with `COVERAGE_PROFILE`:

```bash
COVERAGE=1 make unit-tests-root
go tool cover -func=coverage.profile
go tool cover -html=coverage.profile
```

To reproduce the filtered local coverage used in CI:

```bash
make coverage-local
```

### Integration Tests

List all available integration tests:
```bash
make list-integration-tests
```

Run all integration tests:

```bash
make integration-tests
```

Run a specific integration test (e.g., Fabric IOU test):
```bash
make integration-tests-fabric-iou
```

> [!IMPORTANT]
> `go test` prints nothing for a package that passes, so a ginkgo suite whose filter
> matched **zero specs** reports `ok ... 9.5s` — indistinguishable from real success.
> Never take that as evidence that a focused run exercised anything. Add `-v` and read
> the `Ran N of M Specs` line; `N == 0` means the filter matched nothing.

Enable profiling for deeper analysis:
```bash
export FSCNODE_PROFILER=true
make integration-tests-fabric-iou
```

Enable coverage profiling:
```bash
mkdir -p covdata
GOCOVERDIR=covdata make integration-tests
go tool covdata textfmt -i=covdata -o profile.txt
```

### What CI Gates

CI gates every pull request on the following. It drives the linter through
`golangci-lint-action` and `go fix` through its own workflow rather than through these
targets, so the right-hand column is the local equivalent, not the literal CI command:

| CI check                     | Local equivalent                  |
|------------------------------|-----------------------------------|
| `golangci-lint`, per module  | `make lint`                       |
| `go fix -diff`               | `make go-fix`                     |
| `go mod tidy` leaves no diff | `make tidy`                       |
| unit tests                   | `make unit-tests`                 |
| PostgreSQL unit tests        | `make unit-tests-postgres`        |
| integration suites           | `make integration-tests-<target>` |

If your pull request only touches unit-tested code, `make checks` plus a targeted
`make unit-tests` is usually the fastest high-signal validation pass before pushing.
Before pushing for review, read [Commit Hygiene](workflow.md#commit-hygiene).

## Troubleshooting

### Lint failures in files you never touched

`golangci-lint` keeps one machine-wide cache — `golangci-lint cache status` prints the
directory — shared by every checkout of the repository. Working in more than one
checkout, a second clone or a worktree under `.claude/worktrees/`, can therefore surface
findings against files outside the tree you are linting. The giveaway is a path that
leaves your working directory:

```
../other-worktree/node/start/profile/profile_test.go:119:1: ...
```

Those findings are stale, not real. Clear the cache and re-run:

```bash
golangci-lint cache clean
make checks
```

`cache clean` can take a couple of minutes. Do this before chasing a `make lint`
failure you cannot reproduce by reading the code.

### Missing `FAB_BINS`

If integration tests fail early because Fabric binaries cannot be found, confirm that `FAB_BINS` points to the directory created by `make install-fabric-bins`:

```bash
echo $FAB_BINS
ls $FAB_BINS            # fabric binaries
ls $FAB_BINS/fabric-x   # fabric-x tools
```

### Fabric suites failing in `configtxgen`

Before the fabric-x tools moved into `$FAB_BINS/fabric-x`,
`make install-fabricx-tools` installed its `configtxgen` and `cryptogen` on top
of fabric's. If you ran it against an older checkout, those two binaries in
`$FAB_BINS` are still the fabric-x ones, and every `fabric-*` suite fails while
generating artifacts. `make install-fabric-bins` will not repair it, because it
skips the download whenever `bin/peer version` already matches
`FABRIC_VERSION`. Remove the directory and re-install once:

```bash
rm -rf $(dirname $FAB_BINS)
make install-fabric-bins install-fabricx-tools
```

### Missing chaincode image

A test failing with `chaincode image "fsc-cc/..." not found` means the images
have not been built in this environment:

```bash
make chaincode-images
```

### Missing `ccaas` external builder

`ccaas external builder not found next to FAB_BINS` means `FAB_BINS` points at
a directory whose sibling `builders/ccaas` is absent. `make install-fabric-bins`
alone will not fix it: it skips the download whenever `bin/peer version`
already matches `FABRIC_VERSION`, which is exactly the state you are in.
Remove the directory and re-run the install so it re-downloads the full
release tarball:

```bash
rm -rf $(dirname $FAB_BINS)
make install-fabric-bins
ls $(dirname $FAB_BINS)/builders/ccaas/bin
```

### PostgreSQL unit tests

The PostgreSQL-specific unit tests expect the container image pulled by `make pull-images-database`.
Run that target once before `make unit-tests-postgres` on a new machine.

### Coverage for a single package

When you are improving unit-test coverage for one package, it is often easier to generate coverage for just that package first:

```bash
go test -race -coverprofile=cov.out ./platform/common/utils/dig
go tool cover -func=cov.out
```

## Dependency Management

FSC is a multi-module repository. Use [`scripts/gomate.sh`](../../scripts/gomate.sh) to manage Go dependencies across all modules at once.

Set up a Go workspace so your local editor and tooling resolve cross-module imports correctly:

```bash
./scripts/gomate.sh initwork
```

Tidy all modules after any dependency change:

```bash
./scripts/gomate.sh tidy
```

Update a specific dependency across every module:

```bash
./scripts/gomate.sh update github.com/some/dep@v1.2.3
```

Omit the argument to update all direct dependencies to their latest versions:

```bash
./scripts/gomate.sh update
```

Be careful with updating all dependencies at once. This may easily break something.
Note that `gomate.sh tidy` can also be invoked via `make tidy`. 
After updating, run `make tidy` and `make checks` to verify the result.

## Write Your Own Integration Test

Creating a new integration test is straightforward.
Each test includes a **test harness** and a **network topology** file.

Example:

```bash
mkdir integration/fabricx/helloworld
touch integration/fabricx/helloworld/topology.go
touch integration/fabricx/helloworld/helloworld_test.go
```

- `topology.go` — defines the network topology (organizations, peers, orderers, etc.)
- `helloworld_test.go` — defines the test harness and scenarios

For reference, review existing tests in the [`integration/`](../../integration/) directory.

Run your new integration test:
```bash
make integration-tests-fabricx-helloworld
```
