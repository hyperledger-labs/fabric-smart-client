# Fabric Smart Client
[![License](https://img.shields.io/badge/license-Apache%202-blue)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hyperledger-labs/fabric-smart-client)](https://goreportcard.com/badge/github.com/hyperledger-labs/fabric-smart-client)
[![Go](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/go.yml/badge.svg)](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/go.yml)
[![CodeQL](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/codeql-analysis.yml)

The `Fabric Smart Client` (FSC, for short) is a new Fabric client-side component whose objective is twofold.
1. FSC aims to simplify the development of Fabric-based distributed application by hiding the complexity of Fabric and leveraging 
  Fabric's `Hidden Gems` that too often are underestimated if not ignored.
2. FSC wants to allow developers and/or domain experts to focus on the business processes and not the blockchain technicalities.

# Disclaimer

Fabric Smart Client has not been audited and is provided as-is, use at your own risk. The project will be subject to rapid changes to complete the list of features. 

# Useful Links

- [`Documentation`](./docs/design.md): Discover the design principles of the Fabric Smart Client based on the
`Hidden Gems` of Hyperledger Fabric.
- [`Examples`](./integration/README.md): Learn how to use the Fabric Smart Client via examples. There is nothing better than this.
- [`Feedback`][`fabric-smart-client` Issues]: Your help is the key to the success of the Fabric Smart Client. Submit your issues [`here`][`fabric-smart-client` Issues]
- [`Fabric Token SDK`](https://github.com/hyperledger-labs/fabric-token-sdk): Do you want to develop Token-Based Distributed
application with simplicity and joy? Check our Token SDK out [`here`](https://github.com/hyperledger-labs/fabric-token-sdk).

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

# Use the Fabric Smart Client

## Install

The `Fabric Smart Client` can be downloaded using `go get` as follows: 
 ```
go get github.com/hyperledger-labs/fabric-smart-client
```

The above command clones the repo under `$GOPATH/github.com/hyperledger-labs/fabric-smart-client`. 

We recommend to use `go 1.14.13`. We are testing FSC also against more recent versions of the 
go-sdk to make sure FSC works properly. 

## Makefile

FSC is equipped with a `Makefile` to simplify some tasks. 
Here is the list of commands available.

- `make checks`: check code formatting, style, and licence header.
- `make unit-tests`: execute the unit-tests.
- `make integration-tests`: execute the integration tests. The integration tests use `ginkgo`. Please, make sure that `$GOPATH/bin` is in your `PATH` env variable.
- `make clean`: clean the docker environment, useful for testing.
  
Executes the above from `$GOPATH/github.com/hyperledger-labs/fabric-smart-client`.

## Testing Philosophy

[Write tests. Not too many. Mostly Integration](https://kentcdodds.com/blog/write-tests)

We also believe that when developing new functions running tests is preferable than running the application to verify the code is working as expected.

## Versioning

We use [`SemVer`](https://semver.org/) for versioning. For the versions available, see the [`tags on this repository`](https://github.com/hyperledger-labs/fabric-smart-client/tags).

# License

This project is licensed under the Apache 2 License - see the [`LICENSE`](LICENSE) file for details

[`fabric-smart-client` Issues]: https://github.com/hyperledger-labs/fabric-smart-client/issues