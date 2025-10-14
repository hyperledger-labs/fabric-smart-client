# Fabric Smart Client

[![License](https://img.shields.io/badge/license-Apache%202-blue)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hyperledger-labs/fabric-smart-client)](https://goreportcard.com/badge/github.com/hyperledger-labs/fabric-smart-client)
[![Go](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/tests.yml/badge.svg)](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/go.yml)
[![CodeQL](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/codeql-analysis.yml)

The **Fabric Smart Client (FSC)** is a next-generation client-side framework for Hyperledger [Fabric](https://github.com/hyperledger/fabric) and [Fabric-x](https://github.com/hyperledger/fabric-x).  
It lets you focus on **business logic and distributed workflows**, rather than low-level DLT details.

FSC abstracts away the complexity of a DLT network, enabling developers to build distributed applications with ease.

## Key Features
- **High-level APIs** that abstract away the complexity of interactive distribute applications.
- **Peer-to-peer client overlay** enabling interatcting directly as needed.
- **Advanced transaction orchestration** to implement complex application business processes.
- **Integration-ready with Fabric and Fabric-x networks** via simple configuration, with support for multiple versions.
- **Token SDK** as an example of building distributed ledger applications on top of FSC.

## Quick links
- **Documentation:** [docs/](docs/)
- **Examples and integration tests:** [`integration/fabric/`](integration/fabric/) and [`integration/fabricx/`](integration/fabricx/)  
- **Token SDK:** https://github.com/hyperledger-labs/fabric-token-sdk


## Getting Started

To start developing and testing your application with the Fabric Smart Client:

Ensure you have a working [Go environment](docs/development.md).

Clone the repository:
```bash
git clone https://github.com/hyperledger-labs/fabric-smart-client.git
cd fabric-smart-client
```

## Examples and Integration

The [`integration/`](integration/) directory includes example applications and integration tests for Fabric and Fabric-x networks. 

Start with:
- [`integration/fabric/`](integration/fabric/) — examples and tests for Hyperledger Fabric
- [`integration/fabricx/`](integration/fabricx/) — examples and tests for Hyperledger Fabric-x

These examples demonstrate common FSC patterns, transaction flows, and how to wire the client with a ledger network.

## Contributing

We welcome contributions from everyone. Please read [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines. 

In summary:

1. Fork the repository
2. Create a feature branch
3. Add tests and documentation
4. Submit a Pull Request

Join our community on LFDT Discord -> [#fabric-smart-client](https://discord.gg/hyperledger)

## Versioning

This projects follows [Semantic Versioning (SemVer)](https://semver.org/). See available releases here: https://github.com/hyperledger-labs/fabric-smart-client/tags

## Disclaimer and License

The Fabric Smart Client is provided as-is and has not been formally audited. Use at your own risk. The project is actively developed and APIs may change.

This project is licensed under the **Apache License, Version 2.0 (Apache-2.0)**. Documentation is available under the **Creative Commons Attribution 4.0 International License (CC-BY-4.0)**.
