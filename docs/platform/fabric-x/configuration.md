# Fabric-x Platform Configuration

The Fabric-x platform builds on the shared FSC configuration plus additional `fabric.<network>` service settings for Fabric-x-specific integrations.

## Shared Node Configuration

Start with the common FSC node settings documented in the [shared node configuration](../../configuration.md) guide.

## Fabric-x Services

The Fabric-x platform currently documents configuration for these services:

- `fabric.<network>.notificationService`
- `fabric.<network>.queryService`

Both services support endpoint-based gRPC connectivity and optional TLS or mutual TLS settings, plus a `requestTimeout` for outbound gRPC calls (default `30s`).

### Notification service tuning

`notificationService` configures the client used to subscribe to transaction status updates from the committer's notification service. It is what backs, for example, the finality listeners a view application registers via `fabric.Channel().Committer().AddFinalityListener(txID, listener)`: It subscribes to the transaction on the notification stream and invokes the listener's `OnStatus` callback once the committer reports an outcome.

| Key | Default | What it controls |
|-----|---------|------------------|
| `endpoints[].address` | — | `host:port` of a notification service endpoint. At least one is required. |
| `endpoints[].connectionTimeout` | `30s` | Minimum timeout for establishing the gRPC connection to that endpoint. |
| `endpoints[].tls` | disabled | TLS settings for that endpoint: `enabled`, `rootCerts` (server TLS), plus `clientKey` / `clientCert` (mutual TLS) and `serverNameOverride`. |
| `requestTimeout` | `30s` | How long the committer waits for a subscribed transaction before answering. It is sent to the committer as the subscription's timeout, so the committer reports the outcomes it does have and flags only the still-pending transactions as timed out — rather than the client giving up locally and treating the whole batch as `Unknown`. In practice this is how long a finality listener waits before it is settled. |
| `handlerTimeout` | `5s` | The deadline set on the context handed to a single `OnStatus` callback. Advisory: it cancels the context, but only the callback itself can decide to return. A callback still running past it is reported with a warning. |
| `handlerWorkers` | `16` | How many `OnStatus` callbacks may run concurrently. A concurrency limit, not a rate limit: healthy callbacks return their slots immediately, so far more than this are delivered per second. See the warning below. |
| `handlerQueueSize` | `1000` | How many pending `OnStatus` invocations are buffered while every slot is busy. This is what lets a notification batch larger than `handlerWorkers` be delivered in full. Beyond it, the listener stays registered — with the status the committer sent — and is retried on the next sweep rather than being lost. |
| `listenerTTL` | `2m` | Local backstop: how long a listener may wait without hearing anything at all — a dead or silent stream — before FSC settles it locally with `Unknown`. Keep it comfortably above `requestTimeout`; the committer's timeout is documented non-strict, so a late notification can still arrive after it passes. `listenerTTL: 0` disables the local backstop entirely. |
| `sweepInterval` | `30s` | How often expired listeners are collected and listeners that could not be queued are retried. A listener's worst-case lifetime is `listenerTTL + sweepInterval`. |

Note that `Unknown` does not mean the transaction failed — only that there is no outcome yet for it within the configured bounds. A caller that needs certainty should query the transaction status directly.

> [!WARNING]
> **`OnStatus` must return promptly.** At most `handlerWorkers` callbacks run at
> once. A listener that blocks indefinitely — on a full channel, a stalled store
> call, a contended lock — occupies its slot for as long as it runs.
> `handlerTimeout` cancels the context handed to the callback, but nothing can force
> a callback to return; honoring cancellation is the listener's responsibility.
>
> Once every slot is occupied this way, the queue fills and finality notifications
> stop being delivered on that stream. Listeners that cannot be queued are not lost:
> they stay registered, along with the status the committer sent, and the sweeper
> retries them until a slot frees. Delivery is delayed, not dropped.
>
> This is a deliberate trade: a misbehaving listener degrades notification
> throughput in a bounded, logged way rather than growing goroutines without limit.
> If a deployment has listeners that are legitimately slow, raise `handlerWorkers`
> — but the durable fix is for the listener to hand slow work to its own queue and
> return.
>
> Symptoms to look for in the logs:
>
> - `OnStatus handler for txID=… did not return before its deadline` — a listener is
>   ignoring cancellation and occupying a slot.
> - `deferred N of M finality callbacks` — the queue filled; those listeners are
>   retried on the next sweep.

### Sizing the handler pool

`handlerWorkers` and `handlerQueueSize` do different jobs, and both matter.

`handlerWorkers` bounds how many listeners run at once, which is what keeps a
misbehaving listener from growing goroutines without limit. It is *not* a limit on
throughput: a callback that returns immediately hands its slot straight back, so a
single notification response carrying hundreds of transactions is delivered in full
against the default limit of 16.

`handlerQueueSize` is what makes that true. Notifications arrive in batches, and a
batch is handed to the pool faster than the pool can retire it; the queue holds the
remainder until slots free. Without it, everything past the limit would wait for a
sweep even though the listeners were healthy.

Raise `handlerWorkers` when listeners are legitimately slow and you want more of them
running in parallel. Raise `handlerQueueSize` when notification batches are large and
bursty. If `deferred N of M finality callbacks` appears while listeners are known to be
fast, the queue is the setting to increase: the notifications still arrive, but only
after a sweep tick, so `sweepInterval` becomes the delivery latency.

Complete example:

```yaml
fabric:
  <network>:
    notificationService:
      endpoints:
        - address: 127.0.0.1:5516
          connectionTimeout: 30s
          tls:
            enabled: true
            rootCerts:
              - /path/to/ca.crt
            # for mutual TLS, also set:
            # clientKey: /path/to/client.key
            # clientCert: /path/to/client.crt
      requestTimeout: 30s
      handlerTimeout: 5s
      handlerWorkers: 16
      handlerQueueSize: 1000
      listenerTTL: 2m
      sweepInterval: 30s
```

Invalid timeouts are omitted, in which case the default above applies. Two zero values are meaningful rather than "unset":

- `listenerTTL: 0` disables the local expiry backstop, leaving the committer's
  reply as the only thing that ever settles a listener. The sweeper still runs on
  `sweepInterval`, because it is also what retries callbacks that could not be
  queued.
- `requestTimeout: 0` delegates the subscription timeout to the committer's own
  configuration rather than to the `30s` above.

`handlerTimeout: 0`, `handlerWorkers: 0`, `handlerQueueSize: 0` and `sweepInterval: 0` have no such meaning and fall back to their defaults.

## Related Documentation

- [Fabric-x platform overview](README.md)
- [Fabric platform configuration](../fabric/configuration.md)
