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
| `handlerTimeout` | `5s` | How long a single `OnStatus` callback may run before it is abandoned so it cannot block the dispatcher. Callbacks that ignore context cancellation leak a goroutine. |
| `listenerTTL` | `2m` | Local backstop: how long a listener may wait without hearing anything at all — a dead or silent stream — before FSC settles it locally with `Unknown`. Keep it comfortably above `requestTimeout`; the committer's timeout is documented non-strict, so a late notification can still arrive after it passes. `listenerTTL: 0` disables the local backstop entirely. |
| `sweepInterval` | `30s` | How often expired listeners are collected. A listener's worst-case lifetime is `listenerTTL + sweepInterval`. Ignored when `listenerTTL` is `0`. |

Note that `Unknown` does not mean the transaction failed — only that there is no outcome yet for it within the configured bounds. A caller that needs certainty should query the transaction status directly.

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
      listenerTTL: 2m
      sweepInterval: 30s
```

Invalid timeouts are omitted, in which case the default above applies. Two zero values are meaningful rather than "unset":

- `listenerTTL: 0` disables the local expiry backstop, leaving the committer's
  reply as the only thing that ever settles a listener.
- `requestTimeout: 0` delegates the subscription timeout to the committer's own
  configuration rather than to the `30s` above.

`handlerTimeout: 0` and `sweepInterval: 0` have no such meaning and fall back to their defaults.

## Related Documentation

- [Fabric-x platform overview](README.md)
- [Fabric platform configuration](../fabric/configuration.md)
