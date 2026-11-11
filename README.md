# Fabric Smart Client

[![License](https://img.shields.io/badge/license-Apache%202-blue)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/hyperledger-labs/fabric-smart-client.svg)](https://pkg.go.dev/github.com/hyperledger-labs/fabric-smart-client)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/hyperledger-labs/fabric-smart-client/badge)](https://scorecard.dev/viewer/?uri=github.com/hyperledger-labs/fabric-smart-client)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/4960/badge)](https://www.bestpractices.dev/projects/4960)
[![Tests](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/tests.yml/badge.svg)](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/tests.yml)
[![CodeQL](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/codeql-analysis.yml)
[![Coverage Status](https://coveralls.io/repos/github/hyperledger-labs/fabric-smart-client/badge.svg?branch=main)](https://coveralls.io/github/hyperledger-labs/fabric-smart-client?branch=main)

The Fabric Smart Client (FSC) is a Go framework for building client applications
on Hyperledger [Fabric](https://github.com/hyperledger/fabric) and
[Fabric-x](https://github.com/hyperledger/fabric-x). You write a business
process as a set of _views_, one for each party, that exchange messages over
sessions. FSC handles identities, endorsement, ordering, commit notifications
and the peer-to-peer channels between the parties.

## Features

- A view and session programming model for multi-party protocols.
- Direct peer-to-peer communication between FSC nodes.
- Transaction assembly, endorsement collection and finality tracking for Fabric
  and Fabric-x.
- One node can connect to several networks, each configured in `core.yaml`.

## Getting started

To write your first FSC application:

1. Read [Core concepts](docs/core-concepts.md) for what FSC is and how a
   business process maps to views.
2. Follow the
   [Programming model guide](docs/platform/view/programming-model.md): how an
   application creates a node, installs SDKs, and registers initiator and
   responder views.
3. Add the ledger you target: the
   [Fabric platform](docs/platform/fabric/README.md) or the
   [Fabric-x platform](docs/platform/fabric-x/README.md).
4. Configure your node with [`core.yaml`](docs/configuration.md).

The [documentation index](docs/README.md) lists every guide, and the
[Go API reference](https://pkg.go.dev/github.com/hyperledger-labs/fabric-smart-client)
documents the packages.

## Related projects

- [Panurus](https://github.com/LFDT-Panurus/panurus): a token layer built on
  FSC.
- [Fabric-x Committer](https://github.com/hyperledger/fabric-x-committer): the
  validation and commit service that FSC's Fabric-x platform submits
  transactions to.

## Contributing

We welcome contributions from everyone. Please read
[CONTRIBUTING.md](CONTRIBUTING.md) for the guidelines.

Join our community on the LFDT Discord
[#fabric-smart-client](https://discord.com/channels/905194001349627914/945691888348967012).
We also have a community meeting every Wednesday at 1400 CET on
[Zoom](https://zoom-lfx.platform.linuxfoundation.org/meeting/96953495257?password=0517864f-e5ef-4ef5-89fa-8638e15fddec).
Please see the
[LF Decentralized Trust community calendar](https://zoom-lfx.platform.linuxfoundation.org/meetings/lf-decentralized-trust?view=week)
for details.
[Meeting notes](https://docs.google.com/document/d/13t5-9tfA-7L0Ok4DaK3da_3TpAz5CCd5h5_LryhJyRM/edit?usp=sharing)
are available.

## Versioning

Releases follow [Semantic Versioning](https://semver.org/) and are listed on the
[releases page](https://github.com/hyperledger-labs/fabric-smart-client/releases).
FSC is pre-1.0, so a minor release can change APIs and storage formats; each
minor release has an [upgrade guide](docs/README.md#upgrading).

## Disclaimer and license

The Fabric Smart Client is provided as-is and has not been formally audited. Use
at your own risk. The project is actively developed and APIs may change.

This project is licensed under the **Apache License, Version 2.0 (Apache-2.0)**.
Documentation is available under the **Creative Commons Attribution 4.0
International License (CC-BY-4.0)**.
