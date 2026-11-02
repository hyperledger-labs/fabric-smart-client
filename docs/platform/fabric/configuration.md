# Fabric Platform Configuration

The Fabric platform configuration lives inside the shared FSC `core.yaml` described in the [shared node configuration](../../configuration.md) guide.

## Start with the Shared Configuration

Before adding Fabric-specific sections, configure the common FSC node settings:

- `logging`
- `fsc`
- `grpc`
- `p2p`
- `persistences`
- `web`
- `tracing`
- `metrics`

## Fabric-Specific Settings

When a node uses the Fabric platform, the `core.yaml` also includes `fabric` network definitions and related services such as:

- network connection settings
- identities and MSP material
- finality handling
- vault and persistence wiring
- channel and chaincode access configuration

Use the full [example configuration](../../configuration.md) as the starting point for a Fabric-enabled FSC node.

## Several Networks on One Node

A node can be connected to several networks, of the same platform or of Fabric and Fabric-x, even when their channels have the same name. Network names under `fabric` are local to each node, and another node may call the same network something else. A transaction sent to another node carries a channel key instead: the platform (the network's `driver`), the channel name and the header hash of the channel's block 0. Every member of a channel computes the same hash from its ledger, and two separate networks cannot share it.

The receiving node builds and validates the transaction on the local network that has a configured channel with that key, reading block 0 of the channel the first time it needs the key. This needs no configuration. If no local network matches, or the node cannot read block 0 of a candidate channel yet, it rejects the transaction with an error that names the channel. A transaction without a channel key comes from a node that does not send one; the receiver builds it on the default network and logs a warning once per sender.

## Related Documentation

- [Fabric SDK architecture](fabric-sdk.md)
- [Fabric platform driver architecture](drivers.md)
