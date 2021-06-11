# Secured Asset Transfer

There is nothing better than an example to start learning something new. One of the main goals of FSC is to speed up 
the development of Fabric-based distributed applications by offering a programming model that is closer to the 
business logic rather than the blockchain.

The [Secured asset transfer in Fabric](`https://hyperledger-fabric.readthedocs.io/en/latest/secured_asset_transfer/secured_private_asset_transfer_tutorial.html`) 
is a Fabric sample that demonstrates how an asset can be represented and traded between organizations 
in a Hyperledger Fabric blockchain channel. 

We will start from the above sample to:
- First, show how the `Fabric Smart Client`'s integration infrastructure provides a convenient programmatic 
  environment to test chaincodes-based solutions. Already a huge improvement for the developers.
- Second, highlights the limitations of the current sample and show how these limitations can be overcome
  using the `Fabric Smart Client`'s advanced capabilities and the Hyperledger Fabric hidden gems.
  
Let us start with first bullet, ['here'](./chaincode/README.md). 
There, we will learn:
- The design of the solution used in the sample `Secured asset transfer in Fabric`.
- How to write business views that invoke chaincodes.
- How to write an integration test using the Fabric Smart Client's integration infrastructure.

Then, let us move to the second buller, ['here'](./nochaincode/README.md).
There, we will learn:
- The shortcomings of the current approach to solve the problem.
- How to implement a new solution based on the `Fabric Smart Client`'s advanced capabilities.
- How to write an integration test for the new solution.