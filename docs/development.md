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
```

### Unit Tests

Run all unit tests (with and without race detection):
```bash
make unit-tests
make unit-tests-race
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

Run a specific integration tests (e.g., Fabric IOU test):
```bash
make integration-tests-fabric-iou
```

Enable profiling for deeper analysis:
```bash
export FSCNODE_PROFILER=true
make integration-tests-fabric-iou
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
