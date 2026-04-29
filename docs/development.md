# Development Guide

This document provides guidelines and steps for setting up your local development environment for the **Fabric Smart Client (FSC)**.

For general development best practices, see the following guidelines:  
- [Fabric-x Committer Development Guidelines](https://github.com/hyperledger/fabric-x-committer/blob/main/guidelines.md)
- [Fabric TokenSDK Development Guidelines](https://github.com/hyperledger-labs/fabric-token-sdk/blob/main/docs/development/development.md)

## Prerequisites

Before you begin, ensure you have the following installed:

- **Go** — [Install Go](https://go.dev/doc/install) (see the required version in [`go.mod`](../go.mod))
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
make install-tools install-linter-tool monitoring-docker-images testing-docker-images
```

Platform-specific tools are also required for **Fabric** and **Fabric-x**.

### Fabric

Install the Fabric binaries and Docker images:

```bash
make install-fabric-bins fabric-docker-images 
```

To install a specific Fabric version, set the `FABRIC_VERSION` variable:

```bash
FABRIC_VERSION=3.1.0 make install-fabric-bins
```

The default `FABRIC_VERSION` is defined in the project [Makefile](../Makefile). 


### Fabric-x

Install Fabric-x configuration tools and Docker images:

```bash
make install-fxconfig install-configtxgen fabricx-docker-images 
```

### Set `FAB_BINS`

Most integration tests require Fabric(x) binaries to launch a local test network.
Set the `FAB_BINS` environment variable to point to the directory containing these binaries:

```bash
export FAB_BINS=/home/yourusername/fabric/bin
```

> NOTE: Do *not* store the Fabric binaries inside your fabric-smart-client repository.
Doing so may cause integration tests to fail when installing chaincode.

## Running Tests

FSC includes both unit tests and integration tests.
Integration tests are powered by the NWO (Network Orchestrator), which programmatically creates DLT networks and FSC application nodes.

### Code Checks

Run static analysis and linting:
```bash
make checks
make lint
make lint-auto-fix
make lint-fmt
```

Use `make lint` to run the configured linters on the changes in your branch.
Use `make lint-auto-fix` to apply automatic fixes where `golangci-lint` supports them.
Use `make lint-fmt` to apply the formatter configuration used by CI before pushing a branch.

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

To keep the feedback loop fast while working on a single package, scope the test run with `TEST_PKGS`:

```bash
TEST_PKGS=./platform/common/utils/... make unit-tests
TEST_PKGS=./platform/common/utils/dig go test -race -cover ./platform/common/utils/dig
```

For coverage analysis:
```bash
GO_TEST_PARAMS="-coverprofile=cov.out" make unit-tests
go tool cover -func=cov.out
go tool cover -html=cov.out
```

To reproduce the filtered local coverage used in CI:

```bash
make coverage-local
```

The CI workflow runs:
- `make checks`
- `make lint-fmt`
- `make unit-tests`
- `make unit-tests-postgres`
- `make integration-tests-*`

If you are preparing a pull request that only touches unit-tested code, running `make lint-fmt`, `make checks`, and a targeted `make unit-tests` command is usually the fastest high-signal validation pass before pushing.

### Integration Tests

List all available integration tests:
```bash
make list-integration-tests
```

Run all integration tests:

```bash
make integration-tests
```

Run a specific integration tests (e.g., Fabric IOU test):
```bash
make integration-tests-fabric-iou
```

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

## Troubleshooting Test Setup

### Missing `FAB_BINS`

If integration tests fail early because Fabric binaries cannot be found, confirm that `FAB_BINS` points to the directory created by `make install-fabric-bins`:

```bash
echo $FAB_BINS
ls $FAB_BINS
```

### PostgreSQL unit tests

The PostgreSQL-specific unit tests expect the container image pulled by `make testing-docker-images`.
Run that target once before `make unit-tests-postgres` on a new machine.

### Coverage for a single package

When you are improving unit-test coverage for one package, it is often easier to generate coverage for just that package first:

```bash
go test -race -coverprofile=cov.out ./platform/common/utils/dig
go tool cover -func=cov.out
```

## Write your own integration test

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

For reference, review existing tests in the [`integration/`](../integration/) directory.

Run your new integration test:
```bash
make integration-tests-fabricx-helloworld
```
