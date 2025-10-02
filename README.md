# Fabric Smart Client

[![License](https://img.shields.io/badge/license-Apache%202-blue)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hyperledger-labs/fabric-smart-client)](https://goreportcard.com/badge/github.com/hyperledger-labs/fabric-smart-client)
[![Go](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/tests.yml/badge.svg)](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/go.yml)
[![CodeQL](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/hyperledger-labs/fabric-smart-client/actions/workflows/codeql-analysis.yml)

The `Fabric Smart Client` (FSC, for short) is a new Fabric client-side component whose objective is twofold.

1. To simplify the development of Fabric-based distributed application by hiding the complexity of Fabric and leveraging
  Fabric's `Hidden Gems` that too often are underestimated if not ignored.
2. To allow developers and/or domain experts to focus on the business processes and not the blockchain technicalities.

## Disclaimer

Fabric Smart Client has not been audited and is provided as-is, use at your own risk. The project will be subject to rapid changes to complete the list of features.

## Useful Links

- [`Documentation`](./docs/design.md): Discover the design principles of the Fabric Smart Client based on the
`Hidden Gems` of Hyperledger Fabric.
- `Feedback`: Your help is the key to the success of the Fabric Smart Client.
  - Submit your issues [`here`][`fabric-smart-client` Issues].
  - If you have any questions, queries or comments, find us on [GitHub discussions].

- [`Fabric Token SDK`](https://github.com/hyperledger-labs/fabric-token-sdk): Do you want to develop Token-Based Distributed
application with simplicity and joy? Check our Token SDK out [`here`](https://github.com/hyperledger-labs/fabric-token-sdk).

## Getting started

Clone the code.
Sometimes, we use `$FSC_PATH` to refer to the Fabric Smart Client repository in your filesystem, for example:

```bash
export FSC_PATH=$HOME/myprojects/fabric-smart-client
git clone https://github.com/hyperledger-labs/fabric-smart-client.git $FSC_PATH
```

### Further information

Fabric Smart Client uses a system called `NWO` for its integration tests to programmatically create a fabric network along with the fabric-smart-client nodes. The current version of fabric that is tested can be found in the project [Makefile](https://github.com/hyperledger-labs/fabric-smart-client/blob/main/Makefile) set in the `FABRIC_VERSION` variable.

In order for a fabric network to be able to be created you need to ensure you have downloaded the appropriate version of the hyperledger fabric binaries from [Fabric Releaes](https://github.com/hyperledger/fabric/releases) and unpack the compressed file onto your file system. This will create a directory structure of /bin and /config. You will then need to set the environment variable `FAB_BINS` to the `bin` directory. For example if you unpacked the compressed file into `/home/name/fabric` then you would

```bash
export FAB_BINS=/home/name/fabric/bin
```

Do not store the fabric binaries within your fabric-smart-client cloned repo as this will cause problems running the integration tests as they will not be able to install chaincode.

Almost all the integration tests require the fabric binaries to be downloaded and the environment variable `FAB_BINS` set to point to the directory where these binaries are stored. One way to ensure this is to execute the following in the root of the fabric-smart-client project

```shell
make install-fabric-bins
export FAB_BINS=$PWD/../fabric/bin
```

You can also use this to download a different version of the fabric binaries for example

```shell
FABRIC_VERSION=2.5.0 make install-fabric-bins
```

If you want to provide your own versions of the fabric binaries then just set `FAB_BINS` to the directory where all the fabric binaries are stored.

#### Profiling

FSC comes with built-in profiling support. See more details in [profile.go](https://github.com/hyperledger-labs/fabric-smart-client/blob/main/node/start/profile/profile.go).  
You can enable it using the `FSCNODE_PROFILER` environment variable.

Example:
```bash
FSCNODE_PROFILER=true make integration-tests-iou
```

## Getting Help

Found a bug? Need help to fix an issue? You have a great idea for a new feature? Talk to us! You can reach us on
[Discord](https://discord.gg/hyperledger) in #fabric-smart-client.

## Motivation

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

- With TEE support as offered by [`Fabric Private Chaincode`](https://github.com/hyperledger/fabric-private-chaincode).
- And more...


## Testing Philosophy

[Write tests. Not too many. Mostly Integration](https://kentcdodds.com/blog/write-tests)

We also believe that when developing new functions running tests is preferable than running the application to verify the code is working as expected.

## Versioning

We use [`SemVer`](https://semver.org/) for versioning. For the versions available, see the [`tags on this repository`](https://github.com/hyperledger-labs/fabric-smart-client/tags).

## License

This project is licensed under the Apache 2 License - see the [`LICENSE`](LICENSE) file for details

[`fabric-smart-client` Issues]: https://github.com/hyperledger-labs/fabric-smart-client/issues
[GitHub discussions]: https://github.com/hyperledger-labs/fabric-smart-client/discussions
