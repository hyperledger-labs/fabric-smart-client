# The Fabric SDK

This is the `Fabric SDK` stack:

![img.png](imgs/fabric-sdk.png)

It consists of the following layers:
- `Services` (light-blue boxes): Services offer access to Fabric-specific functionality, such as transaction endorsement, state-based endorsement, and the execution of Chaincode.
- `Fabric API`: This API follows the same abstraction paradigm as the `View API` but provides Fabric-specific functionality to enable `FSC nodes` to communicate with Fabric. Even though this API is specific for Fabric, it allows to abstract away certain details when dealing with a specific Fabric version.
- `Driver API`: This API translates the `Fabric API` to a concrete driver implementation.
- `Driver Implementations`: The Fabric SDK comes with a driver implementation for Fabric V2+.

