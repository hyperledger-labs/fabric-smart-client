# Fabric Platform Driver Architecture

The Fabric platform consists of a [`Fabric API`](https://github.com/hyperledger-labs/fabric-smart-client/tree/main/platform/fabric) 
and a [`Driver API`](https://github.com/hyperledger-labs/fabric-smart-client/tree/main/platform/fabric/driver).
The Fabric API provides a higher level API to deal with a Fabric network. 
It is built on top of the Driver API and its goal is to provider a programming environment that is independent 
from the specific underlying Fabric version used.
On the other hand, the Driver API implementation deals with the technicality of a given Fabric version.

## The `Generic` Driver

The [`Generic` driver](https://github.com/arner/fabric-smart-client/tree/main/platform/fabric/core/generic) can be used with any Fabric version starting from v2.
It can also be further extended by replacing some of the subcomponents. 
For example, one can introduce a different way to handle the chaincode just by provider a different implementation
of the `ChaincodeManager` interface.
