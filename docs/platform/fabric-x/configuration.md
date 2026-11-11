# Fabric-x platform configuration

A Fabric-x node uses the shared FSC `core.yaml`. Every key, including the Fabric-x ones, is listed in the [node configuration reference](../../configuration.md); this page explains the settings of the services that connect a node to the [Fabric-x committer](https://github.com/hyperledger/fabric-x-committer).

## Fabric-x services

The Fabric-x services are configured per network:

- `fabric.<network>.notificationService`
- `fabric.<network>.queryService`
- `fabric.<network>.configMonitor`, see [Channel configuration updates](channelconfig.md)

Both committer services support endpoint-based gRPC connectivity and optional TLS or mutual TLS settings, plus a `requestTimeout` for outbound gRPC calls (default `30s`). For `queryService` it is the deadline of every query, so a zero or negative value falls back to `30s`.

### Endpoint TLS

Each entry of `endpoints` takes a `tls` block that inherits field by field from the network's `fabric.<network>.tls` block (see [Migrating TLS configuration](../../configuration.md#migrating-tls-configuration)). Fabric-x committers require TLS 1.3, so `minVersion` defaults to `772` (TLS 1.3) on these endpoints.

- No TLS: set `enabled: false`, or leave TLS off in both the endpoint's and the network's `tls` block. An endpoint without a `tls` block takes the network's settings.
- Server TLS: set `enabled: true` and `rootCAs.files`; the node verifies the committer's certificate against those CAs.
- Mutual TLS: also set `clientCert.file` and `clientKey.file`.

`serverNameOverride` sets the host name verified in the committer's certificate, which is needed when dialling an IP address:

```yaml
endpoints:
  - address: "10.0.0.5:9090"
    tls:
      enabled: true
      rootCAs:
        files:
          - "/path/to/ca.pem"
      clientCert:
        file: "/path/to/client.pem"
      clientKey:
        file: "/path/to/client-key.pem"
      serverNameOverride: "committer.example.com"
```

### Notification service tuning

`notificationService` configures the client used to subscribe to transaction status updates from the committer's notification service. It is what backs, for example, the finality listeners a view application registers via `ch.Committer().AddFinalityListener(txID, listener)`, with `ch` from `_, ch, err := fabric.GetChannel(sp, network, channel)`. It subscribes to the transaction on the notification stream and invokes the listener's `OnStatus` callback once the committer reports an outcome.

| Key                             | Default  | What it controls                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| ------------------------------- | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `endpoints[].address`           | —        | `host:port` of a notification service endpoint. At least one is required.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `endpoints[].connectionTimeout` | `5s`     | No effect: endpoints are dialled without blocking, and this timeout bounds a blocking dial only.                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `endpoints[].tls`               | disabled | TLS settings for that endpoint: `enabled`, `rootCAs.files` (server TLS), plus `clientKey.file` / `clientCert.file` (mutual TLS) and `serverNameOverride`. Inherits field by field from the network's `tls` block.                                                                                                                                                                                                                                                                                                                                           |
| `requestTimeout`                | `30s`    | How long the committer waits for a subscribed transaction before answering. It is sent to the committer as the subscription's timeout, so the committer reports the outcomes it does have and flags only the still-pending transactions as timed out — rather than the client giving up locally and treating the whole batch as `Unknown`. In practice this is how long a finality listener waits before it is settled.                                                                                                                                     |
| `handlerTimeout`                | `5s`     | The deadline set on the context handed to a single `OnStatus` callback. Advisory: it cancels the context, but only the callback itself can decide to return. A callback still running past it is reported with a warning.                                                                                                                                                                                                                                                                                                                                   |
| `handlerWorkers`                | `16`     | How many `OnStatus` callbacks may run concurrently. A concurrency limit, not a rate limit: healthy callbacks return their slots immediately, so far more than this are delivered per second. See the warning below.                                                                                                                                                                                                                                                                                                                                         |
| `handlerQueueSize`              | `1000`   | How many pending `OnStatus` invocations are buffered while every worker is busy. This is what lets a notification batch larger than `handlerWorkers` be delivered in full. Beyond it, the transaction stays registered — with the status the committer sent — and is retried on the next sweep rather than being lost. Queueing happens per transaction, all of its listeners or none, so keep this comfortably above the most listeners you ever register for a single txID; one with more listeners than the whole queue can only be settled at shutdown. |
| `listenerTTL`                   | `2m`     | Local backstop: how long a listener may wait without hearing anything at all — a silent stream, or one that is down and reconnecting — before FSC settles it locally with `Unknown`. Keep it comfortably above `requestTimeout`; the committer's timeout is documented non-strict, so a late notification can still arrive after it passes. `listenerTTL: 0` disables the local backstop entirely.                                                                                                                                                          |
| `sweepInterval`                 | `30s`    | How often expired listeners are collected and listeners that could not be queued are retried. A listener's worst-case lifetime is `listenerTTL + sweepInterval`.                                                                                                                                                                                                                                                                                                                                                                                            |

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
> - `OnStatus handler for txID=… has not returned within …` — a listener is ignoring
>   cancellation and is still holding a worker. Emitted at the deadline, while the
>   callback is running, so it appears even for one that never returns at all.
> - `deferred N of M finality notifications` — the queue filled; those transactions
>   keep the status the committer sent and are retried on the next sweep.

### Sizing the handler pool

`handlerWorkers` and `handlerQueueSize` do different jobs, and both matter.

`handlerWorkers` bounds how many listeners run at once, which is what keeps a
misbehaving listener from growing goroutines without limit. It is _not_ a limit on
throughput: a callback that returns immediately hands its slot straight back, so a
single notification response carrying hundreds of transactions is delivered in full
against the default limit of 16.

`handlerQueueSize` is what makes that true. Notifications arrive in batches, and a
batch is handed to the pool faster than the pool can retire it; the queue holds the
remainder until slots free. Without it, everything past the limit would wait for a
sweep even though the listeners were healthy.

Raise `handlerWorkers` when listeners are legitimately slow and you want more of them
running in parallel. Raise `handlerQueueSize` when notification batches are large and
bursty. If `deferred N of M finality notifications` appears while listeners are known to be
fast, the queue is the setting to increase: the notifications still arrive, but only
after a sweep tick, so `sweepInterval` becomes the delivery latency.

Complete example:

```yaml
fabric:
  <network>:
    notificationService:
      endpoints:
        - address: 127.0.0.1:5516
          tls:
            enabled: true
            rootCAs:
              files:
                - /path/to/ca.crt
            # for mutual TLS, also set:
            # clientKey:
            #   file: /path/to/client.key
            # clientCert:
            #   file: /path/to/client.crt
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

## Related documentation

- [Fabric-x platform overview](README.md)
- [Fabric platform configuration](../fabric/configuration.md)
