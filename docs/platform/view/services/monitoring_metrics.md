# Metrics Catalog

This document catalogs the metric families currently defined by the Fabric Smart Client (FSC) codebase. For each family, it records the exported name, type, labels, and operational meaning. It focuses on the metrics that FSC itself declares and emits. It excludes the standard Go runtime, process, and Prometheus HTTP handler metrics that exporters expose independently of FSC.

## Overview

The FSC metrics surface has two layers:

- **Explicit metrics**: Counters, gauges, and histograms that are declared directly in service packages.
- **Trace-derived metrics**: Operation counters and duration histograms that are generated automatically by FSC's tracing wrapper whenever FSC code creates a tracer through the tracing provider.

Each tracer base produces two trace-derived families, `*_operations` and `*_duration`.

This catalog is organized by subsystem and records the metric family names that Prometheus sees.

The activation conditions in the current codebase are:

- libp2p transport families appear only when the node uses the libp2p communication stack
- WebSocket transport families appear only when the node uses the WebSocket communication stack
- unary view gRPC request counters appear only on the `ProcessCommand` path
- Fabric ordering, delivery, committer, vault, and finality families appear only when the node executes a Fabric transaction flow
- caller-process tracer families appear only in the process that creates those tracers

Not every family is emitted by the FSC node itself. In the current catalog, the caller-process families are `fsc_view_services_view_client_client_*` and `fsc_view_services_view_client_node_view_client_*`.

## Scope and Conventions

### Export Conditions

FSC exports these metrics when the node runs with:

```yaml
fsc:
  metrics:
    provider: prometheus
```

The exporter is exposed on `/metrics`, served on `fsc.metrics.address` when set and on the FSC web server otherwise. It requires a verified client certificate unless `fsc.metrics.clientAuthRequired` is `false` (see the [configuration reference](../../../configuration.md)). When the metrics provider is disabled, FSC still instantiates the metric objects, but they are backed by a no-op provider and no data is exported.

### Metric Families vs. Time Series

- A counter or gauge family exports one time series for each distinct label combination
- A histogram family exports `*_bucket`, `*_sum`, and `*_count` series for each label combination

### Naming Rules

The full metric name is assembled from:

1. **Namespace**
2. **Subsystem**
3. **Name**

Most FSC packages do not hard-code the namespace or subsystem. Instead, the Prometheus provider derives them automatically from the calling package path. For example:

- package `platform/view/services/view`
- namespace `fsc_view_services`
- subsystem `view`
- metric name `contexts`
- exported metric `fsc_view_services_view_contexts`

Explicit overrides are used in the following places within the current catalog:

- `platform/fabric/core/generic/metrics/metrics.go` registers `fsc_fabric_core_generic_ordering_ordered_transactions`
- `platform/view/services/metrics/connstate/connstate.go` registers the `fsc_connection_*` families

### Labels

Only the labels registered with the metric family are part of the exported schema. For trace-derived metrics, this means that only the label names declared in `tracing.WithMetricsOpts(...)` are guaranteed to appear in Prometheus. A span sets a label from the span attribute with the same key, rendering non-string values as text (`true`, `false`, `3`). Attributes whose key is not a declared label name are recorded on the trace only.

For trace-derived metrics, the namespace and subsystem are also fixed by `tracing.WithMetricsOpts(...)`. If a tracer is created without that option, the wrapper registers the metric family under the tracing package.

### Node Export vs. Library Metrics

The node-facing families are exported by an FSC node when `fsc.metrics.provider = prometheus`. These include:

- the explicit metric families, except `fsc_common_services_grpc_conn_*` (see [Current Caveats](#current-caveats)) and the families of the communication transport the node does not use (libp2p or WebSocket)
- all trace-derived families except `fsc_view_services_view_client_client_*` and `fsc_view_services_view_client_node_view_client_*`

The two caller-process families are exported only by the process that creates those tracers:

- `fsc_view_services_view_client_client_*`
- `fsc_view_services_view_client_node_view_client_*`

When validating the catalog from an FSC node's `/metrics` endpoint, those two caller-process families are expected to be absent unless the node process itself is the tracer owner.

### Operator Summary

For node-level monitoring, the quickest way to interpret the FSC metrics surface is below. Unless `fsc.metrics.clientAuthRequired` is `false`, the scraper must present a client certificate.

- if the node is up and metrics are enabled, expect `fsc_view_services_metrics_operations_fsc_version` on `/metrics`
- if the node is serving views, expect `fsc_view_services_view_contexts`, `fsc_view_services_view_calls_*`, and `fsc_view_services_view_grpc_server_requests_*`, which count view-service commands whether callers reach the view service over gRPC or HTTP
- if the node uses libp2p, expect `fsc_view_services_comm_host_libp2p_bytes_sent` and `fsc_view_services_comm_host_libp2p_bytes_received`
- if the node uses WebSocket transport, expect `fsc_view_services_comm_host_websocket_ws_opened_websockets`, `fsc_view_services_comm_host_websocket_ws_opened_subconns`, and `fsc_view_services_comm_host_websocket_ws_active_subconns`
- if the node participates in Fabric transaction execution, expect the Fabric ordering, committer, delivery, vault, and finality-manager families listed in this catalog
- do not expect `fsc_view_services_view_client_client_*` or `fsc_view_services_view_client_node_view_client_*` on the node unless the node process itself is the tracer owner

If a family is absent, check the matching runtime condition first: communication transport, ingress path, Fabric execution path, or caller-process ownership.

`fsc_connection_state` reports the health of each client connection; see [Connection State](#connection-state) for what each value means and when to alert.

## Explicit Metrics

### Operations

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_view_services_metrics_operations_fsc_version` | gauge | `version` | FSC version information. The gauge is set to `1` at startup for the active version string. |

Primary implementation: `platform/view/services/metrics/operations/metrics.go`, initialized by `platform/view/services/metrics/operations/system.go`.

### View Runtime

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_view_services_view_contexts` | gauge | none | Number of active view contexts currently tracked by the view manager. |

The `contexts` gauge tracks the size of the manager's internal context registry. It increases when initiator or responder contexts are created or registered, and decreases when they are deleted.

Primary implementation: `platform/view/services/view/metrics.go`, updated through the view manager in `platform/view/services/view/manager.go`.

### Comm Service

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_view_services_comm_sessions` | gauge | none | Number of active session entries in the comm layer's session registry. |
| `fsc_view_services_comm_stream_hashes` | gauge | none | Number of distinct stream-hash keys currently stored in the stream cache. |
| `fsc_view_services_comm_active_streams` | gauge | none | Number of registered active stream handlers. |
| `fsc_view_services_comm_opened_streams` | counter | none | Number of newly opened outbound streams. |
| `fsc_view_services_comm_closed_streams` | counter | none | Number of stream-handler closes. |
| `fsc_view_services_comm_stream_handlers` | gauge | none | Number of running stream handler goroutines. |
| `fsc_view_services_comm_dropped_messages` | counter | none | Number of messages dropped because dispatch or enqueue could not complete safely. |
| `fsc_view_services_comm_stream_read_errors` | counter | none | Number of streams dropped because reading a message failed while the node was running. A clean EOF from the remote and streams closed locally are not counted. |

Operational notes:

- `sessions` tracks the size of the session map, not a lifetime total.
- `stream_hashes` tracks distinct cache buckets, not individual streams.
- `opened_streams` is incremented only when FSC creates a fresh outbound stream because the cache could not be reused.
- `dropped_messages` is incremented in two main failure modes:
  - the dispatcher cannot deliver to a closed or closing session
  - the session's queue is full, which also closes the session unless it is the master session

Primary implementation: `platform/view/services/comm/metrics.go`, with updates in `platform/view/services/comm/master.go` and `platform/view/services/comm/p2p.go`.

### Libp2p Transport

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_view_services_comm_host_libp2p_bytes_sent` | counter | `peer`, `protocol` | Total bytes sent on libp2p streams, partitioned by remote peer and protocol ID. |
| `fsc_view_services_comm_host_libp2p_bytes_received` | counter | `peer`, `protocol` | Total bytes received on libp2p streams, partitioned by remote peer and protocol ID. |

These are traffic-volume counters. They count bytes, not messages.

Primary implementation: `platform/view/services/comm/host/libp2p/metrics.go`, updated by `platform/view/services/comm/host/libp2p/bandwidthreporter.go`.

### WebSocket Transport

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_view_services_comm_host_websocket_ws_opened_subconns` | counter | `side` | Number of opened logical sub-connections in the WebSocket multiplexer. |
| `fsc_view_services_comm_host_websocket_ws_closed_subconns` | counter | `side` | Number of closed logical sub-connections in the WebSocket multiplexer. |
| `fsc_view_services_comm_host_websocket_ws_opened_websockets` | counter | `side` | Number of accepted or opened WebSocket connections. |
| `fsc_view_services_comm_host_websocket_ws_active_subconns` | gauge | none | Number of currently active logical sub-connections across all tracked WebSocket clients. |

Operational notes:

- `side` takes the values `client` or `server`.
- `opened_websockets` counts physical WebSocket connections.
- `opened_subconns` counts multiplexed logical channels created on top of those WebSockets.
- `active_subconns` is recomputed periodically by scanning the current connection state.

Primary implementation: `platform/view/services/comm/host/websocket/ws/metrics.go`, updated by `platform/view/services/comm/host/websocket/ws/multiplexed_provider.go`.

### View Service

The view service counts commands on every transport it is served on, gRPC (`fsc.grpc`) and HTTP (`fsc.web`) alike.

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_view_services_view_grpc_server_requests_received` | counter | `command` | Number of unary view-service requests that passed the initial validation and reached the processor dispatch stage. |
| `fsc_view_services_view_grpc_server_requests_completed` | counter | `command`, `success` | Number of unary view-service requests that completed processing. |
| `fsc_view_services_view_grpc_server_requests_rejected` | counter | `reason` | Number of view-service requests rejected because their timestamp is outside the command time window (`stale`) or the command was already accepted (`replay`). |

Operational notes:

- `requests_received` and `requests_completed` are updated only on the unary `ProcessCommand` path; the streaming `StreamCommand` path is not counted by them.
- `requests_rejected` counts both `ProcessCommand` and `StreamCommand`. `stale` is counted before the signature is checked, so any caller can raise it; `replay` is counted only for commands that passed the policy check.
- `command` is the Go type of the command payload, for example `*protos.Command_CallView`.

Primary implementation: `platform/view/services/view/grpc/server/metrics.go`, updated in `platform/view/services/view/grpc/server/server.go`.

### gRPC Connection Metrics

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_common_services_grpc_conn_opened` | counter | none | Number of gRPC connections opened. |
| `fsc_common_services_grpc_conn_closed` | counter | none | Number of gRPC connections closed. |

These counters are implemented by FSC's custom gRPC server stats handler. The handler itself is tested, but the default runtime currently wires the gRPC server through OpenTelemetry's server handler. See [Current Caveats](#current-caveats).

Primary implementation: `platform/common/services/grpc/metrics.go`, with the custom stats handler in `platform/common/services/grpc/serverstatshandler.go`.

### Connection State

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_connection_state` | gauge | `network`, `channel`, `component`, `endpoint` | Current state of a client connection, encoded as below. |
| `fsc_connection_reconnects` | counter | `network`, `channel`, `component`, `endpoint` | Number of reconnect attempts. |
| `fsc_connection_last_connected_timestamp_seconds` | gauge | `network`, `channel`, `component`, `endpoint` | Unix time of the last transition to `connected`. |

`fsc_connection_state` values:

| Value | State | Meaning |
| --- | --- | --- |
| `0` | disconnected | No connection is open. A clean shutdown ends in this state. |
| `1` | connecting | The first connection attempt is in progress. |
| `2` | connected | The connection is established. |
| `3` | reconnecting | A lost connection is being re-established. |
| `4` | stopped | The component gave up retrying. Recovery requires operator action. |

Components that report connection state:

| `component` | `network` / `channel` | `endpoint` | Reported states |
| --- | --- | --- | --- |
| `delivery` | Fabric network and channel | Peer the channel's block stream is attached to, from `fabric.<network>.peers`: the peers with `usage: delivery`, or the peers without a `usage` if none has it | `connecting` on the first connection attempt, `reconnecting` on every later attempt, `connected` once the Deliver stream is open. `stopped` when the committer rejects a block, which stops the channel's block stream (see `commit_failures` below). `disconnected` on a clean shutdown. |
| `orderer` | Fabric network, empty channel | Orderer a broadcast stream is open to, from `fabric.<network>.orderers` or the channel configuration | `connected` when a stream is opened and after each envelope the orderer accepts. `reconnecting` when a broken stream is discarded, including a BFT send that times out or is cancelled. A rejected envelope does not change the state. |
| `fabricx_notification` | Fabric-x network and channel | Notification service endpoint the channel's finality listener manager is connected to, from `fabric.<network>.notificationService.endpoints` | `connecting` on the first stream attempt, `connected` once a notification stream opens, `reconnecting` while the manager backs off before the next attempt, `disconnected` once the node's context ends. |
| `fabricx_query` | Fabric-x network and channel; empty channel for queries made by the ledger | Query service endpoint, from `fabric.<network>.queryService.endpoints` | `connected` after each query the service answers, including with an error, `reconnecting` after a query that fails with `Unavailable`. A query that times out does not change the state. The series is created by the first answered query. |
| `database` | empty | `host:port/dbname` of a Postgres data source, from `fsc.persistences.<name>.opts.dataSource` (see `data_source` below) | `connected` after a successful liveness ping, `reconnecting` after a failed one. Never `disconnected`: pools are never closed, so each pool is pinged every 10 s until the process exits. SQLite reports no state. |

Operational notes:

- Each series is keyed by all four labels, so a node serving several channels or networks has one series per channel for the same endpoint. Group alerts by `network` and `channel` as well as `component`.
- `delivery`, `fabricx_notification` and `database` retry on their own: alert when one of them stays at `reconnecting` longer than you can tolerate, or when `fsc_connection_reconnects` keeps rising. `orderer` and `fabricx_query` change state only on traffic, so on an idle node a series can stay at `reconnecting` with nothing wrong.
- Only Fabric reports `delivery`. A Fabric-x node runs no long-running delivery stream and has no `delivery` series.
- `stopped` means the committer rejected a block, not that the peer is unreachable. Check `commit_failures{class}` and the node log for the cause. A restart re-delivers the same block, so after a `degrade` failure the channel stops again until the cause is fixed.
- Delivery picks a random peer on every connection attempt. When it moves to another peer, the previous peer's series drops to `disconnected`, so at most one peer per channel shows a live state. The attempt is counted against the peer being tried.
- Only the channel's long-running block stream reports. Historical scans (`ScanBlock`, `Scan`) open their own streams and do not report.
- The `orderer` series is updated only by broadcasts. A node that broadcasts nothing keeps the last reported state, and an orderer whose stream broke stays `reconnecting` until a broadcast connects to it again. An orderer no stream was ever opened to has no series. For orderers, `fsc_connection_last_connected_timestamp_seconds` is the time of the last stream opened or envelope accepted.
- A shutdown, including the cancellation of the node's context, ends delivery in `disconnected`. Only a rejected block ends it in `stopped`.
- The `fabricx_query` series is updated only by queries, so it keeps the last reported state while the node queries nothing and is not set to `disconnected` on shutdown. The channel config monitor queries the service once per poll interval. A query service that has never answered has no series, so it never holds `GET /readyz` at 503 as never connected; an unreachable committer shows on `fabricx_notification` instead.
- `GET /readyz` on the web listener is derived from the same state: it returns 503 while a component (network, channel, component) has not been `connected` through any endpoint since startup, or while any connection is `stopped`. See [configuration](../../../configuration.md).

Primary implementation: `platform/view/services/metrics/connstate/connstate.go`, updated in `platform/fabric/core/generic/delivery/delivery.go`, `platform/fabric/core/generic/ordering/`, `platform/fabricx/core/finality/nlm.go`, `platform/fabricx/core/committer/queryservice/query.go` and `platform/view/services/storage/driver/sql/postgres/dbmetrics.go`.

### Database Pool

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_database_open_connections` | gauge | `data_source` | Number of established connections, in use and idle. |
| `fsc_database_in_use` | gauge | `data_source` | Number of connections in use. |
| `fsc_database_idle` | gauge | `data_source` | Number of idle connections. |
| `fsc_database_wait_count` | gauge | `data_source` | Total number of times a caller waited for a connection. |
| `fsc_database_wait_duration_seconds` | gauge | `data_source` | Total time callers waited for a connection. |

The values are `sql.DBStats` of each Postgres connection pool, sampled every 10 s. `wait_count` and `wait_duration_seconds` only grow; alert on their rate. The liveness ping keeps one connection open, so `open_connections` stays at 1 or more. When `maxOpenConns` is set and every connection is in use, the ping is skipped and the state stays `connected`.

The `data_source` label is `host:port/dbname` of the first host in the data source; for a unix socket, `host` is the socket directory, and a data source that cannot be parsed is labeled `unknown`. Pools whose data sources differ only in user or connection options share the label and therefore one series.

Primary implementation: `platform/view/services/storage/driver/sql/postgres/dbmetrics.go`.

### Fabric Ordering

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_fabric_core_generic_ordering_ordered_transactions` | counter | `network` | Number of transactions that were successfully accepted by the ordering service. |

The CFT broadcaster increments this counter after the orderer returns `Status_SUCCESS`. The BFT broadcaster increments it once per envelope, after enough orderers return `Status_SUCCESS` to meet the threshold.

Primary implementation: `platform/fabric/core/generic/metrics/metrics.go`, updated in `platform/fabric/core/generic/ordering/cft.go` and `platform/fabric/core/generic/ordering/bft.go`.

### Fabric Committer

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_fabric_core_generic_committer_notify_status` | histogram | none | Duration of publishing transaction status change events. |
| `fsc_fabric_core_generic_committer_notify_finality` | histogram | none | Duration of notifying local finality listeners. |
| `fsc_fabric_core_generic_committer_post_finality` | histogram | none | Duration of posting a finality event into the finality manager. |
| `fsc_fabric_core_generic_committer_handler` | histogram | `status` | Duration of the committer's per-transaction handler execution. |
| `fsc_fabric_core_generic_committer_block_commit` | histogram | none | End-to-end duration of committing the transactions in a block. |
| `fsc_fabric_core_generic_committer_event_queue` | histogram | none | Time spent enqueueing a finality event into the committer's internal event channel. |
| `fsc_fabric_core_generic_committer_event_queue_length` | gauge | none | Current number of pending events in the committer's event queue. |
| `fsc_fabric_core_generic_committer_commit_retries` | counter | `network`, `channel` | Number of block commit attempts retried after a transient failure. |
| `fsc_fabric_core_generic_committer_commit_failures` | counter | `network`, `channel`, `class` | Number of block commit failures returned to the caller, which stop that channel's block stream. A commit stopped by a shutdown is not counted. |

Operational notes:

- `notify_finality` measures the synchronous notification of local waiting listeners.
- `post_finality` measures the cost of posting the event to the shared finality manager queue.
- `notify_status` measures the cost of publishing `TransactionStatusChanged` events.
- `handler{status}` uses the values:
  - `not_found`
  - `failure`
  - `successful`
- `event_queue` measures enqueue blocking time, not the total residence time of an event in the queue.
- `event_queue_length` is a manually maintained occupancy gauge.
- `commit_retries` rising while `commit_failures` stays flat is a channel absorbing
  transient faults and recovering on its own. It is worth a dashboard, not a page.
  While the database is unreachable the committer keeps retrying the same block
  until the database comes back, so a long outage shows up as `commit_retries`
  rising for as long as it lasts, not as a `commit_failures` increment. Which
  Postgres errors count as an unreachable database is listed in
  [Postgres errors](../../../configuration.md#postgres-errors). To page on
  a long outage, alert on the `database` connection state staying `reconnecting`,
  or on `commit_retries` rising for longer than the outage you tolerate.
- **Any** increment of `commit_failures` means that channel has stopped ingesting
  blocks and will not resume without intervention. A stopped channel emits no
  further blocks and no further errors, so it does not show up in `block_commit` at
  all - that histogram simply stops receiving observations, which is
  indistinguishable from an idle channel. This counter is what an alert should
  watch.
- `commit_failures{class}` uses the values:
  - `degrade` - a deterministic failure on an already-final block, such as a
    configuration this node cannot apply. Retrying cannot clear it.
  - `fatal` - an invariant the node relies on is broken, so continuing would risk
    committing over untrustworthy state.
  - `retry` never appears here: a retryable failure that exhausts its budget is
    wrapped in `ErrRetriesExhausted`, which classifies as `degrade`, so an alert on
    `degrade` catches the exhausted-contention case as well as the deterministic
    ones.
- A commit that stops because the delivery stops or the node shuts down is not
  counted in `commit_failures`: the block is committed again once delivery resumes.
- Both counters carry `network` and `channel`, because the metrics provider is
  process-wide and a node serves several channels: without them every channel would
  increment one series and an operator would learn only that something stopped.

Primary implementation: `platform/fabric/core/generic/committer/metrics.go`, updated in `platform/fabric/core/generic/committer/committer.go`.

### Vault

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_common_core_generic_vault_commit` | histogram | none | Average per-transaction commit duration inside a vault batch commit. |
| `fsc_common_core_generic_vault_batched_commit` | histogram | none | Total wall-clock time experienced by a `CommitTX` caller, including batching overhead. |

Operational notes:

- `commit` records the batch duration normalized by the number of transactions in the batch.
- `batched_commit` records the end-to-end duration of the outer `CommitTX` call.

Primary implementation: `platform/common/core/generic/vault/metrics.go`, updated in `platform/common/core/generic/vault/vault.go`.

## Trace-Derived Metrics

FSC wraps the tracing provider so that every tracer can export two metric families:

- `<metric-base>_operations`: counter of completed spans
- `<metric-base>_duration`: histogram of completed span durations

Unless otherwise stated, the duration is recorded in seconds and uses the default Prometheus histogram buckets.

### View and API Tracers

| Metric Base | Labels | Description |
| --- | --- | --- |
| `fsc_view_services_view_calls` | `success`, `view`, `initiator_view` | Local view execution spans created by the view runtime around each view invocation started by the FSC view manager. |
| `fsc_view_services_view_grpc_server_view_service` | `success` | View gRPC service tracer declared by the gRPC server package. No span start was found on the current `ProcessCommand` or `StreamCommand` implementation paths. |
| `fsc_view_services_view_client_client` | none | Outbound view service invocation tracer, over gRPC or HTTP. This family is emitted by the caller process, not by the remote FSC node that handles the request. |
| `fsc_view_services_view_client_node_view_client` | `fid` | Local client tracer used when a node invokes a view through the in-process local client helper. |

Each metric base above exports:

- `<metric-base>_operations`
- `<metric-base>_duration`

Primary implementation:

- `platform/view/services/view/context.go`
- `platform/view/services/view/client/client.go`
- `platform/view/services/view/client/local.go`
- `platform/view/services/view/grpc/server/server.go`
- `platform/view/services/view/grpc/server/view.go`

### Comm and Transport Tracers

| Metric Base | Labels | Description |
| --- | --- | --- |
| `fsc_view_services_comm_host_websocket_ws_multiplexed_ws` | `context_id` | WebSocket multiplexer tracer for inbound view invocation handling. |

Primary implementation: `platform/view/services/comm/host/websocket/ws/multiplexed_provider.go`.

### Fabric Tracers

| Metric Base | Labels | Description |
| --- | --- | --- |
| `fsc_fabric_core_generic_delivery_delivery` | `type` | Delivery service tracer for received messages from Fabric deliver. |
| `fsc_fabric_core_generic_committer_commits` | none | Committer tracer for `commit_block` and `commit_tx` spans. |
| `fsc_fabric_core_generic_committer_committer` | none | Committer tracer used for operations such as waiting for finality and reloading config transactions. |

The delivery tracer uses the label `type`, with the currently observed values:

- `unknown`
- `block`
- `status`
- `other`

Primary implementation:

- `platform/fabric/core/generic/delivery/delivery.go`
- `platform/fabric/core/generic/committer/metrics.go`
- `platform/fabric/core/generic/committer/committer.go`

### Common Tracers

| Metric Base | Labels | Description |
| --- | --- | --- |
| `fsc_common_core_generic_vault_vault` | none | Vault tracer for batch commit operations. |
| `fsc_common_core_generic_committer_finality_listener_manager` | none | Finality listener manager tracer for listener dispatch. |
| `fsc_common_core_generic_committer_finality_manager` | none | Finality manager tracer for status polling and event posting logic. |

Primary implementation:

- `platform/common/core/generic/vault/vault.go`
- `platform/common/core/generic/committer/listenermgr.go`
- `platform/common/core/generic/committer/finality.go`

## Label Reference

The following labels are part of the current FSC metric surface.

| Label | Meaning | Typical Values |
| --- | --- | --- |
| `channel` | Fabric channel name | deployment-specific channel name |
| `class` | Committer commit-failure class that stopped the channel's block stream | `degrade`, `fatal` |
| `command` | gRPC command payload type handled by the view service | `*protos.Command_CallView` |
| `component` | FSC component that owns a client connection | `orderer`, `delivery`, `fabricx_notification`, `fabricx_query`, `database` |
| `context_id` | FSC context identifier used by the WebSocket multiplexer tracer | FSC context UUID or caller-provided context ID |
| `data_source` | Postgres data source of a connection pool, without credentials | `host:port/dbname` |
| `endpoint` | Remote address of a client connection | `host:port`; `host:port/dbname` for `database` |
| `fid` | View factory identifier used by the local view client tracer | application-defined factory ID |
| `initiator_view` | Initiator view identifier associated with a child view execution | fully qualified view identifier or empty |
| `network` | Fabric network name | deployment-specific network name |
| `peer` | Remote libp2p peer identifier | peer ID string |
| `protocol` | Libp2p protocol identifier | protocol ID string |
| `reason` | Why the view service rejected a request | `stale`, `replay` |
| `side` | WebSocket connection side | `client`, `server` |
| `status` | Committer handler outcome classification | `not_found`, `failure`, `successful` |
| `success` | Success flag used by `fsc_view_services_view_calls_*`, `fsc_view_services_view_grpc_server_requests_completed`, and `fsc_view_services_view_grpc_server_view_service_*` | `true`, `false`, or an empty value where the current implementation declares the label but does not populate it |
| `type` | Delivery message type classification | `unknown`, `block`, `status`, `other` |
| `version` | FSC version string | node version |
| `view` | Fully qualified FSC view identifier | Go package and type-based identifier |

## Current Caveats

The following families have implementation caveats:

- `fsc_connection_*`: the `database_notifier` component reports no connection state, so no series are exported for it.
- `fsc_view_services_view_grpc_server_view_service_*`: the tracer is declared, but no span start was found in the current code path, so these families are expected to remain idle.
- `fsc_view_services_comm_host_websocket_ws_closed_subconns`: the counter family is declared, but no increment site was found in the current production code.
- `fsc_common_services_grpc_conn_opened` and `fsc_common_services_grpc_conn_closed`: FSC's custom handler for these counters is tested, but the default runtime wiring currently uses OpenTelemetry's gRPC server handler instead of the FSC-specific one.
- `fsc_view_services_view_client_client_*`: these are caller-process metrics. The standard integration harness creates its external view clients with `tracing.NewProviderFromConfig(...)`, not with the metrics-wrapping tracing provider, so these families are not expected on the FSC node's `/metrics`.
- `fsc_view_services_view_client_node_view_client_*`: these are emitted only when the in-process local client helper is used inside the node. They are not part of the normal external gRPC or web ingress path.

