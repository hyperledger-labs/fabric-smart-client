# Fabric Smart Client
[![License](https://img.shields.io/badge/license-Apache%202-blue)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hyperledger-labs/fabric-smart-client)](https://goreportcard.com/badge/github.com/hyperledger-labs/fabric-smart-client)
[![Go](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/go.yml/badge.svg)](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/go.yml)
[![CodeQL](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/codeql-analysis.yml)

The `Fabric Smart Client` (FSC, for short) is a new Fabric client-side component whose objective is twofold.
1. To simplify the development of Fabric-based distributed application by hiding the complexity of Fabric and leveraging 
  Fabric's `Hidden Gems` that too often are underestimated if not ignored.
2. To allow developers and/or domain experts to focus on the business processes and not the blockchain technicalities.

# Disclaimer

Fabric Smart Client has not been audited and is provided as-is, use at your own risk. The project will be subject to rapid changes to complete the list of features. 

# Useful Links

- [`Documentation`](./docs/design.md): Discover the design principles of the Fabric Smart Client based on the
`Hidden Gems` of Hyperledger Fabric.
- [`Samples`](./samples/README.md): A collection of sample applications that demonstrate the use of the Fabric Smart Client. 
- `Feedback`: Your help is the key to the success of the Fabric Smart Client. 
  - Submit your issues [`here`][`fabric-smart-client` Issues].
  - If you have any questions, queries or comments, find us on [GitHub discussions].

- [`Fabric Token SDK`](https://github.com/hyperledger-labs/fabric-token-sdk): Do you want to develop Token-Based Distributed
application with simplicity and joy? Check our Token SDK out [`here`](https://github.com/hyperledger-labs/fabric-token-sdk).

# Getting started

Clone the code and make sure it is on your `$GOPATH`.
(Note: To set up your go development environment see the [official go documentation](https://go.dev/doc/gopath_code#GOPATH))
Sometimes, we use `$FSC_PATH` to refer to the Fabric Smart Client repository in your filesystem.

```bash
export FSC_PATH=$GOPATH/src/github.com/hyperledger-labs/fabric-smart-client
git clone https://github.com/hyperledger-labs/fabric-smart-client.git $FSC_PATH
```

You are ready to run the [`samples`](./samples/README.md) in `$FSC_PATH`.

# Getting Help

Found a bug? Need help to fix an issue? You have a great idea for a new feature? Talk to us! You can reach us on
[Discord](https://discord.gg/hyperledger) in #fabric-smart-client.

# Motivation

[Hyperledger Fabric]('https://www.hyperledger.org/use/fabric') is a permissioned, modular, and extensible open-source 
DLT platform. Fabric architecture follows a novel `execute-order-validate` paradigm that supports distributed 
execution of untrusted code in an untrusted environment. Indeed, Fabric-based distributed applications can 
be written in any general-purpose programming language.

Developing applications for Hyperledger Fabric is often hard, sometimes painful. Fabric is a very powerful 
ecosystem whose building blocks must be carefully orchestrated to achieve the desired results. Currently, 
the Fabric Client SDKs are too limited. They do not offer any advanced capabilities to let the developers 
focus on the `application business processes`, and harness the full potential of Fabric.

What would happen if the developers could use a `Smart(er) Fabric Client` that offers:
- A high level API that hides the complexity of Fabric;
- A Peer-to-Peer infrastructure that let Fabric Clients and Endorsers talk to each as required by the business processes;
- Advanced transaction orchestration;
- A simplified model to interact with chaincodes;
- A State-based programming model that let you forget about RW sets and focus on business objects and their interactions? 

Developing Fabric-based distributed applications would become simpler and joyful.
If you are a domain expert, the Fabric Smart Client hides the complexity of Fabric and allows you to focus on the business interactions.
If you are a skilled software engineer, you will be able to leverage the full power of Fabric.

But, this is not all. The Fabric Smart Client is a client-side component that can be used to develop applications: 
- Based on other backends like [`Orion`](https://github.com/hyperledger-labs/orion-server).
- With interoperability using the [`Weaver`](https://github.com/hyperledger-labs/weaver-dlt-interoperability) framework.
- With TEE support as offered by [`Fabric Private Chaincode`](https://github.com/hyperledger/fabric-private-chaincode). 
- And more...

Explore our [`Samples`](./samples/README.md) to see how you can use the Fabric Smart Client to develop your own applications.

# Testing Philosophy

[Write tests. Not too many. Mostly Integration](https://kentcdodds.com/blog/write-tests)

We also believe that when developing new functions running tests is preferable than running the application to verify the code is working as expected.

# Versioning

We use [`SemVer`](https://semver.org/) for versioning. For the versions available, see the [`tags on this repository`](https://github.com/hyperledger-labs/fabric-smart-client/tags).

# License

This project is licensed under the Apache 2 License - see the [`LICENSE`](LICENSE) file for details

[`fabric-smart-client` Issues]: https://github.com/hyperledger-labs/fabric-smart-client/issues
[GitHub discussions]: https://github.com/hyperledger-labs/fabric-smart-client/discussions
