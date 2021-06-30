## Examples via Integration Tests

Integration tests are useful to show how multiple components work together.
The Fabric Smart Client comes equipped with some of them to show the main features.
To run the integration tests, you need to have Docker installed and ready to be used.

Each integration test bootstraps the FSC and Fabric networks as needed, and initiate the
business processes by invoking the `initiator view` on the specific FSC nodes.

Here is a list of available examples:

- [`Ping Pong`](./fsc/pingpong/README.md): A simple ping-pong between two FSC nodes to start with the basics.
- [`I Owe You`](./fabric/iou/README.md): In this example, we orchestrate a simple
  `I Owe You` use case between a `lender` and a `borrower`, mediated by an `approver`.
  Moreover, we will learn more about the `State-Based Programming model`.
- [`Secured Asset Transfer`](./fabric/atsa/README.md): 
  In this example, our starting point is the [`Secured asset transfer in Fabric`](https://hyperledger-fabric.readthedocs.io/en/release-2.2/secured_asset_transfer/secured_private_asset_transfer_tutorial.html) 
  sample. 
  The objectives are manifolds: 
  - First, to show how the `Fabric Smart Client`'s integration infrastructure provides a convenient programmatic environment to test
     chaincodes-based solutions. Already a huge improvement for the developers.
  - Second, to highlights the limitations of the current sample and show how these limitations can be overcome 
    using the `Fabric Smart Client`'s advanced capabilities.
    
