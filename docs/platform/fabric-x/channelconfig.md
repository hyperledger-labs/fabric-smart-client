# Channel configuration updates

A Fabric-x channel's configuration changes over time, for example when an
organization joins or the orderer endpoints change. An FSC node picks these
changes up with a channel configuration monitor
(`platform/fabricx/core/channel/config`): it polls the committer's query service
for the latest configuration transaction and, when it finds a newer one, updates
the channel's membership service and ordering service. This applies to Fabric-x
networks only.

## Lifecycle

The Fabric-x channel provider starts one monitor per channel when it opens the
channel and stops it when the channel is closed. No caller action is needed.
Every monitor of a network shares the network's `configMonitor` settings.

The monitor keeps the last processed configuration in memory only. After a
restart it checks the current configuration and applies it.

## How a change is detected and applied

On start the monitor checks immediately, then once every `pollInterval`. Each
check:

1. calls `GetConfigTransaction` on the query service;
2. compares the reported version, and the config sequence in the envelope's own
   content, with the last processed ones;
3. if either is newer, calls `Update(envelope)` on the membership service,
   extracts the orderer configuration with `OrdererConfig`, and passes the
   consensus type and orderers to `Configure` on the ordering service;
4. records the new version and sequence.

The sequence check means a committer that reports a stale version cannot hide a
configuration whose content has advanced. If the sequence cannot be read from
the envelope, the monitor logs a warning and relies on the version alone.

A check that fails (query service unavailable, an empty configuration
transaction, a failed membership or ordering update) is retried with exponential
backoff: the first retry waits `initialRetryDelay`, each later one doubles the
wait up to `maxRetryDelay`, and at most `maxRetries` retries follow the first
attempt. When they are exhausted the monitor logs the error and waits for the
next poll. Closing the channel cancels a pending retry and stops the monitor.

## Configuration

The monitor is configured per network under `fabric.<network>.configMonitor`,
listed with every other key in the
[node configuration reference](../../configuration.md). Every key is optional:

| Key                 | Default | Validation                                      |
| ------------------- | ------- | ----------------------------------------------- |
| `pollInterval`      | `1m`    | must be positive                                |
| `maxRetries`        | `5`     | must be non-negative; `0` disables retries      |
| `initialRetryDelay` | `1s`    | must be positive and not exceed `maxRetryDelay` |
| `maxRetryDelay`     | `5m`    | must be positive                                |

A value that fails validation stops the channel from opening with an error that
names the key, such as `pollInterval must be positive`.

```yaml
fabric:
  mynetwork:
    configMonitor:
      pollInterval: 10s
      maxRetries: 10
      initialRetryDelay: 500ms
      maxRetryDelay: 1m
    channels:
      - name: mychannel
```

The keys can be overridden from the environment like any other key (see
[Overriding configuration keys](../../configuration.md#overriding-configuration-keys)),
for example `CORE_FABRIC_MYNETWORK_CONFIGMONITOR_POLLINTERVAL=30s`.
