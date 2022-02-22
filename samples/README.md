# Samples

Samples are a collection of small and simple apps that demonstrate how to use the library.

To run the samples, we recommend to use go 1.16 or later. You will also need docker when using Fabric. 
To make sure you have all the required docker images, you can run `make docker-images` in the 
folder `$GOPATH/src/github.com/hyperledger-labs/fabric-smart-client`.

- [`I Owe You`](./fabric/iou/README.md): In this example, we orchestrate a simple
  `I Owe You` use case between a `lender` and a `borrower`, mediated by an `approver`.
- [`Fabric to Fabric Interoperability view Weaver Relay`](./fabric/weaver/relay/README.md): In this example, we see how to use
  the [weaver relay infrastructure](https://labs.hyperledger.org/weaver-dlt-interoperability/docs/external/architecture-and-design/relay) 
  to perform queries between two Fabric networks.
- [`Institutional Review Board (IRB) Sample`](https://github.com/hyperledger/fabric-private-chaincode/tree/main/samples/demos/irb):
  This demos shows how to use Fabric Private Chaincode in combination with Fabric Smart Clients.

The samples will make use of docker to run various components. To make sure you have all the docker images you need,
please, run `make docker-images` in the folder `$GOPATH/src/github.com/hyperledger-labs/fabric-smart-client`.

## Additional Examples via Integration Tests

Integration tests are useful to show how multiple components work together.
The Fabric Smart Client comes equipped with some of them to show the main features.
To run the integration tests, you need to have Docker installed and ready to be used.

Each integration test bootstraps the FSC and Fabric networks as needed, and initiate the
business processes by invoking the `initiator view` on the specific FSC nodes.

Here is a list of available integration tests:

- [`Ping Pong`](../integration/fsc/pingpong/README.md): A simple ping-pong between two FSC nodes to start with the basics.
  Moreover, we will learn more about the `State-Based Programming model`.
- [`Secured Asset Transfer`](../integration/fabric/atsa/README.md):
  In this example, our starting point is the [`Secured asset transfer in Fabric`](https://hyperledger-fabric.readthedocs.io/en/release-2.2/secured_asset_transfer/secured_private_asset_transfer_tutorial.html)
  sample.
  The objectives are manifolds:
    - First, to show how the `Fabric Smart Client`'s integration infrastructure provides a convenient programmatic environment to test
      chaincodes-based solutions. Already a huge improvement for the developers.
    - Second, to highlights the limitations of the current sample and show how these limitations can be overcome
      using the `Fabric Smart Client`'s advanced capabilities.
- [`Fabric Private Chaincode: Echo`](../integration/fabric/fpc/echo/README.md): In this example, we show how to invoke a Fabric
  Private Chaincode called [`Echo`](https://github.com/hyperledger/fabric-private-chaincode/tree/main/samples/chaincode/echo).