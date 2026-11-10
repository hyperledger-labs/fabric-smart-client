# Fabric Platform Configuration

A Fabric node's settings live in the `fabric` section of the shared FSC `core.yaml`, next to the `logging` and `fsc` sections every node has. Every key is listed in the [node configuration reference](../../configuration.md); this page covers how a node with several networks matches a transaction to one of them.

## Several Networks on One Node

A node can be connected to several networks, of the same platform or of Fabric and Fabric-x, even when their channels have the same name. Network names under `fabric` are local to each node, and another node may call the same network something else. A transaction sent to another node carries a channel key instead: the platform (the network's `driver`), the channel name and the header hash of the channel's block 0. Every member of a channel computes the same hash from its ledger, and two separate networks cannot share it.

The receiving node builds and validates the transaction on the local network that has a configured channel with that key. If two local networks match, as when one network is configured under two names, the default network wins; if neither is the default, the transaction is rejected. This needs no configuration. If no local network matches, or the node cannot read block 0 of a candidate channel yet, it rejects the transaction with an error that names the channel. A transaction without a channel key comes from a node that does not send one; the receiver builds it on the default network and logs a warning once per sender.

A node reads block 0 of a channel the first time it needs the key and keeps the key in memory. It waits at most 10 seconds for the read; after a failed or timed-out read it reports the failure for 5 seconds before it reads block 0 again. A node does not send a transaction to another node while the key of its channel cannot be read, because the receiver would otherwise build it on its default network.

## Related Documentation

- [Fabric SDK architecture](fabric-sdk.md)
- [Fabric platform driver architecture](drivers.md)
