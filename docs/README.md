# Fabric Smart Client documentation

## Getting started

To write your first FSC application:
1. Read [Core concepts](core-concepts.md) for what FSC is and how a business process maps to views.
2. Follow the [Programming model guide](platform/view/programming-model.md): how an application creates a node, installs SDKs, and registers initiator and responder views.
3. Add the ledger you target: the [Fabric platform](platform/fabric/README.md) or the [Fabric-x platform](platform/fabric-x/README.md).
4. Configure your node with [`core.yaml`](configuration.md).

## Core concepts

- [Overview and architecture](core-concepts.md)
- [Shared node configuration](configuration.md)

## Platforms

- [View platform](platform/view/README.md)
- [Fabric platform](platform/fabric/README.md)
- [Fabric-x platform](platform/fabric-x/README.md)

Each platform section includes a local `README.md` plus a `configuration.md` page as a starting point.

## Development

- [Contributor guide](../CONTRIBUTING.md)
- [Development guide](dev/development.md): setup, building, running tests, CI
- [Architecture and dependency injection](dev/architecture.md)
- [Code conventions](dev/conventions.md)
- [Contribution workflow](dev/workflow.md)
- [Commit signing (DCO + GPG)](dev/signing.md)
- [Rebasing onto upstream main](dev/rebasing.md)
- [Resolving merge conflicts](dev/merge-conflicts.md)
- [Writing documentation](dev/documentation.md): where each page lives and how it is written
- [Releasing](dev/releasing.md) (maintainers)

## Testing

- [Testing guide](dev/testing.md): unit tests, mocks, fixtures, fuzzing
- [Mocks and fakes](dev/mocks.md)
- [Integration tests](dev/integration-tests/README.md): the suites, their targets, and running them
- [Authoring integration tests](dev/integration-tests/authoring.md)
- [Fault-recovery tests](dev/integration-tests/fault-recovery.md)
- [Benchmark examples](../integration/benchmark/README.md)

Coding agents start from [`AGENTS.md`](../AGENTS.md).

## Reference

- [Go API reference (pkg.go.dev)](https://pkg.go.dev/github.com/hyperledger-labs/fabric-smart-client)
- [FSC CLI reference](reference/fsc-cli.md)
- [Node CLI reference](reference/node-cli.md)

## Upgrading

A node or database older than v0.26.0 is upgraded to v0.26.x first, then to later releases.
Releases whose release data allows it are upgraded node by node; see
[Rolling upgrades](upgrade/rolling-upgrade.md).

- [Upgrading from v0.13 to v0.26](upgrade/v0.13-to-v0.26.md)
- [Upgrading from v0.26 to v0.27](upgrade/v0.26-to-v0.27.md)
- [Upgrading from v0.27 to v0.28](upgrade/v0.27-to-v0.28.md)
- [Upgrading from v0.28 to v0.29](upgrade/v0.28-to-v0.29.md)
