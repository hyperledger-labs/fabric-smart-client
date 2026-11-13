# Chaguar platform driver

## Overview

The **Fabric Smart Client (FSC)** includes support for
[Chaguar](https://github.com/Chaguar/chaguar), available under
[`platform/fabricx`](../../../platform/fabricx). The platform's Go packages,
driver, configuration and integration test targets are named `fabricx`.

The Chaguar platform driver builds on the existing Fabric platform
implementation and replaces only the components necessary for Chaguar-specific
functionality, such as the transaction processing component.

## Application node

To use the Chaguar platform within an FSC application node, you need to install
the Chaguar [SDK](../../../platform/fabricx/sdk/dig/sdk.go):

```go
import (
	"log"

	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

func main() {
	node := fscnode.New()
	if err := node.InstallSDK(fabricx.NewSDK(node)); err != nil {
		log.Fatalf("failed installing SDK: %v", err)
	}
	node.Execute(func() error {
		registry := view.GetRegistry(node)
		// register your views, for example:
		return registry.RegisterFactory("init", &MyInitiatorViewFactory{})
	})
}
```

### Configuration

The Chaguar services are configured under `fabric.<network>` in `core.yaml`:
the keys are listed in the
[node configuration reference](../../configuration.md), and
[Chaguar platform configuration](configuration.md) covers notification service
tuning and endpoint TLS.

## Use Chaguar from a view

Views use the same APIs on Chaguar as on Fabric: the transaction, endorsement
and finality views of
[`platform/fabric/services/state`](../../../platform/fabric/services/state) and
[`platform/fabric/services/endorser`](../../../platform/fabric/services/endorser),
and `fabric.GetDefaultChannel` and `fabric.GetChannel` to reach a channel. Start
with [Use Fabric from a view](../fabric/README.md#use-fabric-from-a-view) and
install `fabricx.NewSDK` instead of the Fabric SDK. The Chaguar IOU example,
[`integration/fabricx/common/iou`](../../../integration/fabricx/common/iou),
runs the views of the Fabric IOU example unchanged. Notable differences:

- Finality comes from the committer's
  [notification service](#notification-service-transaction-finality), and the
  query service answers status lookups.
- `ch.ACLProvider().CheckACL` checks a signed proposal against the channel's
  policies; on Fabric it returns `driver.ErrNotImplemented`.
- `ch.Committer().ProcessNamespace` has no effect.
- The vault's query executor does not support range queries (`GetStateRange`).

## Notification service (transaction finality)

The notification service delivers realtime transaction finality events to FSC
application nodes. It maintains a persistent bidirectional gRPC stream to the
[Chaguar committer](https://github.com/Chaguar/committer)'s
`SidecarService` (`OpenNotificationStream`), which pushes notifications as
transactions are committed, invalidated, or timed out. The connection to the
committer is re-used and shared among all active listeners for a given network
and channel.

### API

Register through the channel's committer — `ch.Committer()`, the same API every
FSC driver exposes — and use:

- `AddFinalityListener(txID, listener)`: registers a `FinalityListener` whose
  `OnStatus` callback fires once when the transaction reaches finality. Only the
  first listener for a given txID triggers a subscription request to the
  committer; subsequent listeners for the same txID piggyback on it. After
  dispatch, all listeners for the txID are automatically removed. Registering
  the same listener instance twice for one txID is a no-op.
- `RemoveFinalityListener(txID, listener)`: unregisters a listener before
  delivery. Safe to call for unknown txIDs, unregistered listeners, and
  listeners already delivered to.

The `OnStatus` callback receives one of the validation codes of
[`platform/fabric/driver`](../../../platform/fabric/driver/committer.go):
`driver.Valid` (committed), `driver.Invalid` (rejected), or `driver.Unknown`
(undetermined / timeout).

The committer's manager is the notification-stream manager wrapped in a
`finality.ResolvingListenerManager`, which asks the query service before
subscribing. That is what makes registering for an _already final_ transaction
report its status instead of `Unknown`: the notification stream reports each
transaction once, so a subscription created after that point would never be
answered. `finality.GetListenerManager(sp, network, channel)` returns the
unwrapped manager and does not do this.

> [!WARNING]
> `OnStatus` implementations MUST observe `ctx.Done()` and return
> promptly, handing slow work to their own queue. At most `handlerWorkers`
> (default 16) callbacks run at once, and once every slot is held by a callback
> that does not return, finality notifications stop being delivered. See
> [notification service tuning](configuration.md#notification-service-tuning)
> for the failure mode and its log messages.

### Limitations

- **Reconnection keeps pending listeners**: when the stream breaks, the manager
  reopens it with jittered exponential backoff (100ms up to 10s) until the node
  shuts down. Listeners stay registered across the outage, and the new stream
  requests every pending txID again. `AddFinalityListener` during an outage
  succeeds; the transaction is requested once the stream is back. `listenerTTL`
  keeps running, so a listener waits at most `listenerTTL + sweepInterval`
  whether or not the stream recovers. Only node shutdown settles what is still
  pending, with `Unknown`. The committer reports only statuses that arrive after
  a subscription, so it answers a re-requested transaction that committed while
  the stream was down only as timed out, once `requestTimeout` passes. Once the
  channel's committer has wrapped it, the manager also asks the query service
  for the status of the pending transactions each time the stream fails or a
  connection attempt fails, and once a new stream has re-requested them; the
  attempts are spaced by the jittered backoff above. The manager is shared per
  channel, so this holds for every listener on it, including those registered
  through `finality.GetListenerManager`. A transaction the query service reports
  as committed or invalid is settled with that status, and with the query
  service's message if it gives one, without waiting for `requestTimeout`, even
  while the stream is still down. A stream that stops delivering without failing
  triggers no query until the gRPC keepalive (`keepalive` in the
  `notificationService` config, off by default) detects the dead connection and
  the stream fails; without keepalive, only `listenerTTL` bounds that wait. A
  query that fails, for instance because the committer is down altogether, is
  logged as a warning and retried on the next attempt, and `listenerTTL` still
  bounds the wait. A timed-out `Unknown` from the committer is still re-resolved
  from the query service.
- **Bounded handler concurrency**: at most `handlerWorkers` (default 16)
  callbacks run concurrently, buffered by a queue of `handlerQueueSize` (default
  1000). When every worker is busy and the queue is full, a transaction stays
  registered with the status the committer sent and is retried on the next
  sweep, so delivery is delayed rather than lost; see
  [notification service tuning](configuration.md#notification-service-tuning).
- **Shutdown does not wait on stuck handlers**: when the node shuts down,
  everything still registered is queued for settlement, the queue is closed so
  the workers finish the backlog, and the wait is abandoned after a single
  `handlerTimeout` — one deadline for the whole teardown, not one per listener.
  A callback still inside an unresponsive listener can outlive the manager, so a
  single bad listener cannot hang node shutdown; at most `handlerWorkers` such
  callbacks exist, since each occupies one worker. Listeners queued behind one
  are not settled — nothing can force such a callback to return.

### Configuration

The notification service is configured under
`fabric.<network>.notificationService`: its endpoints, their TLS,
`requestTimeout` and the handler pool settings. See
[notification service tuning](configuration.md#notification-service-tuning).

## Query service

The query service provides synchronous access to ledger state by connecting to
the [Chaguar committer](https://github.com/Chaguar/committer)'s
`Query` service over gRPC. It is used for operations such as retrieving
transaction status and channel configuration.

### Configuration

The query service is configured under `fabric.<network>.queryService`, with the
same endpoint and TLS settings as the notification service (see
[endpoint TLS](configuration.md#endpoint-tls)). Its `requestTimeout` is the
deadline of every query.

## Several committers

An organization can run several committers, and a node lists every one of them
under `notificationService.endpoints` and `queryService.endpoints`. The node
dials every endpoint and uses the first one that is not failing, so when its
committer goes down it moves to another without a restart:

| Path                                                                                           | Moves                                                                   |
| ---------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Queries (transaction status, channel configuration)                                            | after a call fails with `Unavailable`, retried once on another endpoint |
| Notification stream (finality)                                                                 | when the stream reconnects                                              |
| Ledger API (`GetBlockByNumber`, `GetBlockNumberByTxID`, `GetTransactionByID`, `GetLedgerInfo`) | after a call fails with `Unavailable`, retried once on another endpoint |

A call made while every committer is down fails with the error of the last
attempt.

## Integration tests

The Chaguar integration tests run a mock orderer and one committer per
organization in containers.
[Chaguar committers in integration tests](../../dev/integration-tests/fabricx-committers.md)
covers the committer containers, their settings and an example topology.

## Channel configuration updates

A node picks up channel configuration changes, such as a new organization or new
orderer endpoints, by polling the query service; see
[Channel configuration updates](channelconfig.md).

## Additional resources

- Chaguar Repository: <https://github.com/Chaguar/chaguar>
- Chaguar Committer: <https://github.com/Chaguar/committer>
