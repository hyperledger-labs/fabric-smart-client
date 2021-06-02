# Fabric Smart Client
[![License](https://img.shields.io/badge/license-Apache%202-blue)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hyperledger-labs/fabric-smart-client)](https://goreportcard.com/badge/github.com/hyperledger-labs/fabric-smart-client)

The `Fabric Smart Client` (FSC, for short) is a new Fabric client-side component whose objective is twofold.
On one side, FSC aims to simplify the development of Fabric-based distributed application by hiding the complexity of Fabric and leveraging the Fabric `hidden gems` that too often are underestimated if not ignored.
On the other side, FSC aims to allow developers and/or domain experts to focus on the business processes and not the blockchain technicalities.

# Disclaimer

Fabric Smart Client has not been audited and is provided as-is, use at your own risk. The project will be subject to rapid changes to complete the list of features. 

# Motivation

[Hyperledger Fabric]('https://www.hyperledger.org/use/fabric') is a permissioned, modular, and extensible open-source DLT platform. Fabric architecture follows a novel `execute-order-validate` paradigm that supports distributed execution of untrusted code in an untrusted environment. Indeed, Fabric-based distributed applications can be written in any general-purpose programming language.

Developing applications for Hyperledger Fabric is often hard, sometimes painful. Fabric is a very powerful ecosystem whose building blocks must be carefully orchestrated to achieve the desired results. Currently, the Fabric Client SDKs are too limited. They do not offer any advanced capabilities to let the developers focus on the `application business processes`, and harness the full potential of Fabric.

What would happen if the developers could use a `Smart(er) Fabric Client` that offers:
- A high level API that hides the complexity of Fabric;
- A Peer-to-Peer infrastructure that let Fabric Clients and Endorsers talk to each as required by the business processes;
- Advanced transaction orchestration;
- A simplified model to interact with chaincodes;
- A State-based programming model that let you forget about RW sets and focus on business objects and their interactions? 

Developing Fabric-based distributed applications would become simpler and joyful.
If you are a domain expert, the Fabric Smart Client hides the complexity of Fabric and allows you to focus on the business interactions.
If you are a skilled software engineer, you will be able to leverage the full power of Fabric.

## Fabric and its Hidden Gems

A Fabric network consists of a set of network nodes. As Fabric is permissioned, all network nodes have an identity. These nodes take up one of three roles:
- `Clients` submit `transaction proposals` for execution (or simulation), help orchestrate the execution phase, and, finally, broadcast transactions for ordering.
- `Peers` execute transaction proposals and validate transactions. All peers maintain the blockchain ledger, an append-only data structure recording all transactions in the form of a hash chain, as well as the state, a succinct representation of the latest ledger state. Not all peers execute all transaction proposals, only a subset of them called endorsing peers (or, simply, endorsers) does, as specified by the policy of the chaincode to which the transaction pertains.
- `Ordering Service Nodes (OSN)` are the nodes that collectively form the ordering service. In short, the ordering service establishes the total order of all transactions in Fabric, where each transaction contains state updates and dependencies computed during the execution phase, along with cryptographic signatures of the endorsing peers.

In a Fabric network, sta

Fabric Nodes interact to achieve pre-defined goals.
Let's give a quick look at the transaction lifecycle, and the interactive protocols used
by Fabric nodes. It can be split in three phases:

- `Execution Phase`: In the execution phase, clients sign and send a transaction proposal (or, simply, proposal) to one or more endorsers for execution. The endorsers simulate the proposal, by executing the operation of the specified chaincode, which has been installed on the blockchain. After the simulation, the endorser cryptographically signs a message called endorsement, which contains readset and writeset (RWSet, for short) (together with metadata such as transaction ID, endorser ID, and endorser signature) and sends it back to the client in a proposal response. The client collects endorsements until they satisfy the endorsement policy of the chaincode, which the transaction invokes.
- `Ordering Phase`: When a client has collected enough endorsements on a proposal, it assembles a transaction and submits this to the ordering service.
- `Validation Phase`: Blocks are delivered to peers either directly by the ordering service or through gossip. A new block then enters the validation phase which consists of three sequential steps: (i) The evaluation of the endorsement policy occurs in parallel for all transactions within the block, (ii) A read-write conflict check is done for all transactions in the block sequentially, (iii) The ledger update phase runs last, in which the block is appended to the locally stored ledger, and the blockchain state is updated.

Notice that, each phase is essentially an interactive protocol, executed by given parties, used to achieve a specific goal.
A few remarks on the above protocol will help the reader to understand the reasons why we decided to revise it, and the directions we want to follow:
- `Each party should be able to run its own business logic`: The endorsement policy does not say anything about the process that should be used to assemble the RWSet. In particular, it does not say that all endorsers of a given chaincode should run the same business logic. Nor it says that the RWSet should be produced by endorsers. Indeed, the execution phase can be implemented in other ways. For example, the clients could assemble the RWSet directly and then ask the endorsers to `approve` the transaction.
- `Free players`: Fabric nodes have a precise role in the network. In other words, Fabric nodes are not interchangeable. This means that a client cannot endorser, an endorser cannot submit transactions to the ordering service, and so on. Indeed, an endorser is just a network node that possesses a signing secret key accepted by some endorsement policy.

The above remarks guided us during the design of the Fabric Smart Client.

# Design (High-Level)

The `Fabric Smart Client` (FSC, for short) is the `Next-Generation Fabric Client SDK`. FSC simplifies the development of Fabric-based distributed applications by putting at the centre the `business processes`, and by `hiding the unnecessary burden` of dealing with Fabric.

A `business process` is a collection of `related and structured activities` carried by `business parties` in which a specific sequence of steps serves a particular `business goal`. In other words, a business process is an `interactive protocol` among `business parties` each of which has its own `business view` (or simply `view`) of the process. A `view` is then the sequence of steps that a business party must execute to serve a particular `business goal`.
FSC aims to offer a comfortable environment to build such interactive protocols backed by the Hyperledger Fabric blockchain.

FSC is founded on the following key ingredients to build interactive protocols:
- `View`: A distributed application consists of multiple interactive protocols. An interactive protocol represents a computation jointly carried on by multiple parties in a distributed fashion. Those parties communicate by establishing communication channels or sessions. The view of each party in an interactive protocol is what we call `View`. We call `initiator` the party that starts the interactive protocol. The initiator does that by executing the view representing her in that interactive protocol. When a view opens a communication session to another party, this party executes, in response, the view representing her role in the interaction.
  Therefore, views are unit of computation that can be used to agree on the content of a transaction, to disseminate data, to perform key agreement, to deliver a transaction to an ordering service, and so on.
- `Identity`: Each party has at least one `identity`. To keep this concept as general as possible, we defined an identity as a container of a lower level identity. A lower level identity can be anything meaningful enough to represent the intuitive concept of a digital identity. For example, it can be an X509 certificate, an ECDSA public-key, an Idemix identity, and so on. An identity can be used to produce digital signatures, to identify the party that a view want to open a communication session to, and so on.
- `Session`: One of the main goals of the Fabric Smart Client is to help developers designing distributed applications. The core of any distributed application is its communication layer. Therefore, FSC provides a communication abstraction that we call `Session`. A session is a bidirectional communication channel, with a unique identifier, between two parties identified by their identities. Let us now describe the lifecycle of a session with an example. Alice and Bob are two parties who know each other and who wants to interact to accomplish a given task. Alice is the initiator and starts the interactive protocol by executing the view representing her in the protocol. At some point, Alice needs to send a message to Bob. To do that, Alice opens a session to Bob. Notice that, we are assuming that Alice knows already Bob’s identity. When Alice’s message reaches Bob, Bob responds by executing the view representing him in protocol. Bob gets Alice’s message by first obtaining the session Alice’s opened. Bob can use the very same session to respond to Alice and the ping-pong can continue until Alice and Bob reach their goal.

We call `FSC Node` a network node that runs the FSC stack. FSC nodes form an `FSC network`. This network lives alongside one or more Fabric networks. FSC Node can connect to each other in a peer-to-peer fashion.

The Fabric Smart Client consists of `platforms` or `modules`. Each platform exposes a coherent set of API to address specific tasks     
FSC backbone :
- The `View` platform is the core of FSC. It offers API and services to allow FSC nodes:
  -- To connect to each other in a peer to peer fashion.
  -- To manage and execute business views.
- The `Fabric` platform builds on top of the `View` platform and offers API and services to allow FSC nodes to communicate with Fabric. The Fabric module is not just the usual Fabric Client SDK, it is more. Indeed, we can identify the following components that make the Fabric module different from the current Fabric Client SDK:
  -- `Chaincode API`: These are APIs that allow the developer to invoke any chaincode and assemble Fabric transactions as she would do with the Fabric Client SDK.
  -- `Identity Management`: The Fabric platform allows an FSC node to play the role of an `Endorser`, not just that of a `Fabric Client`.
  -- `State-Based Programming Model API` (`SPM`, for short): Instead of dealing directly with a Read-Write Set (RWS, for short), the SPM API allows the developer to think directly in terms of business objects. The SPM API takes care of translating these business objects to a well-formed RWS. Therefore, FSC nodes can collaboratively assemble a Fabric Transaction and its RWS, and then other FSC nodes, playing the role of endorsers, endorse the assembled transaction. This gives much more flexibility than the default chaincode-centric protocol to assemble transactions, that Fabric offers.
  -- `Vault`: The Fabric platform equips an FSC node with a local storage system that contains only the transactions the node is interested in or helped to assemble. An FSC node does not need to store the entire ledger but only what is relevant. This storage space allows the FSC nodes to keep temporary version of the transactions they are assembling before they get submitted for ordering.
  
# Issues

The `Fabric Smart Client` issues are tracked in the GitHub issues tab.

# Use the Fabric Smart Client

## Install

The `Fabric Smart Client` can be downloaded using `go get` as follows: 
 ```
go get github.com/hyperledger-labs/fabric-smart-client
```

The above command clones the repo under `$GOPATH/github.com/hyperledger-labs/fabric-smart-client`. 

We recommend to use `go 1.14.13`. We are testing FSC also against more recent versions of the go-sdk to make sure FSC works properly. 

## Examples via Integration Tests

Integration tests are useful to show how multiple components work together. 
The Fabric Smart Client comes equipped with some of them to show the main features.
To run the integration tests, you need to have Docker installed and ready to be used.

Each integration test bootstraps the FSC and Fabric networks, and commands specific Fabric Smart Client node, the initiators, to initiate given business processes.

Here is a list of available examples:

- [`Secured Asset Transfer`](integration/fabric/atsa/nochaincode/README.md): In this example, we start from the [`Secured asset transfer in Fabric`](https://hyperledger-fabric.readthedocs.io/en/release-2.2/secured_asset_transfer/secured_private_asset_transfer_tutorial.html) sample and show how this can be implemented used the Fabric Smart client with different flavors.
- [`I Owe You`](integration/fabric/iou/README.md): In this example, we orchestrate a simple `I Owe You` use case between a lender and a borrower.

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