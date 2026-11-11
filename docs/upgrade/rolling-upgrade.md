# Rolling upgrades

An application built on FSC usually spans several organizations, and each one upgrades its own nodes on its own schedule. This page is the procedure for one organization that replaces its nodes' binaries while the network keeps running and other organizations stay on their release. The guide of each release lists what changes in it; read it first.

## Which releases

A release says how it is upgraded in its release data, `upgrade.mode` in `docs/upgrade/releases/<version>.json` (see [Release data](releases/README.md)). This procedure applies only to a release whose mode is `rolling`. A release whose mode is `stop-all`, such as v0.28.0, is upgraded by stopping every node of the network before any node runs it, as its upgrade guide describes.

## Supported skew

A node works with nodes of its own release and of the release before it, over P2P, the view API and a shared database. Keep every node on the latest release or the one before it. Nodes further apart are not supported. No check at startup refuses them; they fail at the first session or store they have no common version for.

Two nodes agree per session on the format version of each FSC component they both support (see [Format negotiation](../platform/view/services/comm/readme.md#format-negotiation)), so there is nothing to switch on once a node is upgraded. A node uses the new formats with peers that support them and the old ones with peers that do not. A release guide names the changes that negotiation does not cover. Those need every organization involved to upgrade in the same window.

## Procedure

1. **Back up every database the node uses.** A rollback past a contract step, or past a layout conversion of step 1, needs the backup (see [Schema migrations](../platform/view/services/schema-migrations.md#first-start-after-an-upgrade)).
2. **Check with the new binary first.** Run it with the node's `core.yaml` and key material while the node still runs the old one (see [Before rolling out a new binary](../platform/view/services/schema-migrations.md#before-rolling-out-a-new-binary)):

   ```bash
   ./mynode node migrate --check
   ./mynode node peers --check
   ```

   `node peers --check` reports a peer of a release without version ranges as `no preamble` and exits 0.
3. **Replace the binaries one node, and one replica, at a time.** A node stopped with SIGTERM lets its running views finish before it exits (see [Graceful shutdown](../platform/view/services/shutdown.md)). The new binary migrates its stores at first start. On a database the previous release wrote, it holds contract steps back, so replicas still on the previous release keep working against the same stores. On libp2p, the replicas of a node share one peer ID and its streams, so a session one replica negotiated can reach another: upgrade the replicas of a libp2p node together, or drain the old ones first, unless the release guide says otherwise.
4. **Do not create stores while a replica of the previous release shares the database.** A store the new binary opens for the first time, such as the vault of a channel the node joins or a store under a new table prefix, runs its contract steps at once. A replica of the previous release then fails on what the new binary writes there. Join channels and add stores once every replica runs the new release.
5. **Run contract steps once no node of the organization runs the previous release,** if the release has any: `node migrate --contract` (see [Contract steps](../platform/view/services/schema-migrations.md#contract-steps)). Then restart every replica, one at a time, so that each one writes the new forms.

## Rollback

Until a contract step has run, a node rolls back by starting the previous binary on the same database. After one has run, rolling back means restoring the backup of step 1. A store the new binary created (step 4) ran its contract steps when it was created, so the previous binary cannot read it either.

## How FSC tests this

The `fabric-mixedversion` integration suite runs nodes of the previous release next to nodes of the current tree. It checks that a node of one release endorses the transactions of the other, that a node keeps reading and updating its stores while its replicas are upgraded and rolled back one at a time on a shared database, and that transactions keep committing while a responder's replicas behind a load balancer are upgraded and rolled back one at a time, with at most one failed transaction per restart and none of the failed ones committed. A release is marked `rolling` only after this suite has run against its previous release. See [Integration Tests](../dev/integration-tests/README.md).

The release guides are listed under [Upgrading](../README.md#upgrading).
