# Chaincode -> FSC Example (Asset Transfer)

## Topology

This example uses a single process to simulate the tutorial roles below:

* Client (CLI)
* FSC Node (initiator flow)
* Endorser (responder flow)
* Fabric-X (ordering + finality)

### Flow diagram

```text
Client (CLI)
  |
  v
FSC Node (initiator flow)
  |-- ReadAsset(id)
  |-- validate transfer
  |-- prepare transaction
  |-- collect endorsement
  |-- submit transaction
  |-- verify state
  v
Endorser (responder flow)
  |
  v
Fabric-X (ordering + finality)
```

## API

The example follows the same asset-transfer shape as Fabric chaincode:

* `ReadAsset(id string)`
* `TransferAsset(id, newOwner string)`

`ReadAsset` reads the current asset state. `TransferAsset` drives the FSC flow, submits the transaction, and verifies the updated state after finality.

These functions mirror the Fabric asset-transfer chaincode, but are implemented using FSC views instead of on-ledger chaincode execution.

## Execution

Run the example with:

```bash
go run ./cmd/token-transfer --asset-id a1 --new-owner bob
```

## Example output

```text
reading asset
validating transfer
preparing transaction (phase: pre-submission)
endorsing transaction
submitting transaction (phase: post-submission)
verifying state
success: asset a1 now owned by bob
```

Each line corresponds to a stage in the FSC transaction lifecycle.

## Notes

* The roles are simulated in a single process.
* In this example, client and endorser roles are simulated within a single process for simplicity, but the flow structure matches how FSC nodes would interact in a real network.
* The example stays tutorial-style and intentionally small.
* The code mirrors the chaincode asset-transfer API without extra abstractions.

## Future Work

This example currently simulates all roles in a single process for simplicity.

The next step is to extend this into a real multi-node FSC + Fabric-X setup where:

* separate FSC nodes act as clients and endorsers
* communication happens over networked views
* ordering and finality are handled by a real Fabric-X deployment