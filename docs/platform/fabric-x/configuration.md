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

The finality listener manager backing `notificationService` accepts various timeout configurations. They control the local expiry backstop that settles a finality listener with `Unknown` if the committer never notifies it (see `platform/fabricx/core/finality/nlm.go`):

| Key | Default | What it controls |
|-----|---------|------------------|
| `requestTimeout` | `30s` | Sent to the committer as the outbound notification request's timeout, so it gives up and replies once it passes rather than the client aborting locally and marking transactions the committer may already know the outcome of as `Unknown`. |
| `handlerTimeout` | `5s` | How long a single `OnStatus` callback may run before it is abandoned. |
| `listenerTTL` | `2m` | How long a listener may wait for a notification before being settled locally with `Unknown`. Set it comfortably above `requestTimeout` — that timeout is documented non-strict, so a late notification can still arrive after it passes. Setting `listenerTTL: 0` explicitly disables local expiry. |
| `sweepInterval` | `30s` | How often expired entries are collected. Worst-case entry lifetime is `listenerTTL + sweepInterval`. |

Example:

```yaml
fabric:
  <network>:
    notificationService:
      endpoints:
        - address: 127.0.0.1:5516
      requestTimeout: 30s
      handlerTimeout: 5s
      listenerTTL: 2m
      sweepInterval: 30s
```

Omitting any of these keys keeps the default shown above. Only `listenerTTL: 0` is treated specially — it disables local expiry rather than falling back to the default.

## Related Documentation

- [Fabric-x platform overview](README.md)
- [Fabric platform configuration](../fabric/configuration.md)
