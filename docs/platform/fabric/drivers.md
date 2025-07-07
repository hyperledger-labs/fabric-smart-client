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

### Create a new Driver from `Generic`

Imagine you want to create a new driver for a new version of Fabric under development, let's call it `FabricDEV`.
Here are the key differences between Fabric 2.0+ and FabricDEV:
1. A new endorser transaction format with a new RW set format, and
2. No chaincode support.

We need the following:
1. A customized vault that can marshall and unmarshall the new RW set format. 
This can be done by setting custom populator and marshaller to the existing vault. 
Look [`here`](fabricdev/core/fabricdev/vault/vault.go) for example of a vault instantiation.
2. A customized `Transaction Manager` to handle the new transaction format. 
Look [`here`](fabricdev/core/fabricdev/transaction/manager.go).
3. A customized implementation of the `Ledger` to get access to remote ledger without the use of chaincodes.
Look [`here`](fabricdev/core/fabricdev/ledger/ledger.go) for an example.
To use it, we need a new channel provider that uses the new ledger implementation.
Look [`here`](fabricdev/core/fabricdev/channelprovider.go) for an example.

Once your driver is ready, you can create a new SDK to register your driver as shown here [`sdk.go`](fabricdev/sdk/dig/sdk.go).
