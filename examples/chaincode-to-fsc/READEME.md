# Chaincode to FSC POC (Asset Transfer)

## Overview

This example demonstrates how a simple Hyperledger Fabric chaincode use case (asset transfer) can be implemented using a Fabric Smart Client (FSC)-style flow.

## Key Idea

In Fabric:

* business logic runs inside chaincode

In FSC:

* business logic is executed as a client-side flow

## Flow

1. Read asset
2. Validate request
3. Prepare transaction
4. Simulate endorsement
5. Submit transaction
6. Verify updated state

## Example

```bash
cd examples/chaincode-to-fsc
go run ./cmd/token-transfer --asset-id a1 --new-owner bob
```

## Output

```
reading asset
validating transfer
preparing transaction
endorsing transaction
submitting transaction
verifying state
transfer successful: a1 -> bob
```

## Scope

* Single asset transfer
* Single org
* Mocked backend (in-memory)

## Future Work

* Integrate real FSC components
* Add multi-org endorsement flow
* Extend to full token/asset lifecycle
