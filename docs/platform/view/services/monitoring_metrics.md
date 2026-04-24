# Metrics Catalog

This document catalogs the metric families currently defined by the Fabric Smart Client (FSC) codebase. For each family, it records the exported name, type, labels, and operational meaning. It focuses on the metrics that FSC itself declares and emits. It excludes the standard Go runtime, process, and Prometheus HTTP handler metrics that exporters expose independently of FSC.

## Overview

The FSC metrics surface has two layers:

- **Explicit metrics**: Counters, gauges, and histograms that are declared directly in service packages.
- **Trace-derived metrics**: Operation counters and duration histograms that are generated automatically by FSC's tracing wrapper whenever FSC code creates a tracer through the tracing provider.

At the time of writing, the repository defines:

- **29 explicit metric families**
- **14 tracer bases**, which produce **28 trace-derived metric families** (`*_operations` and `*_duration`)

This catalog is organized by subsystem and records the metric family names that Prometheus sees.

The activation conditions in the current codebase are:

- libp2p transport families appear only when the node uses the libp2p communication stack
- WebSocket transport families appear only when the node uses the WebSocket communication stack
- unary view gRPC request counters appear only on the `ProcessCommand` path
- web view client tracer families appear only when a view is invoked through the FSC web endpoint
- Fabric ordering, delivery, committer, vault, and finality families appear only when the node executes a Fabric transaction flow
- caller-process tracer families appear only in the process that creates those tracers

Not every family is emitted by the FSC node itself. In the current catalog, the caller-process families are `fsc_view_services_view_grpc_client_client_*` and `fsc_view_services_view_grpc_client_node_view_client_*`.

## Scope and Conventions

### Export Conditions

FSC exports these metrics when the node runs with:

```yaml
fsc:
  metrics:
    provider: prometheus
```

The exporter is exposed through the FSC web server on `/metrics`. When the metrics provider is disabled, FSC still instantiates the metric objects, but they are backed by a no-op provider and no data is exported.

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

Explicit overrides are used in the following place within the current catalog:

- `platform/fabric/core/generic/metrics/metrics.go` registers `fsc_fabric_core_generic_ordering_ordered_transactions`

### Labels

Only the labels registered with the metric family are part of the exported schema. For trace-derived metrics, this means that only the label names declared in `tracing.WithMetricsOpts(...)` are guaranteed to appear in Prometheus.

For trace-derived metrics, the namespace and subsystem are also fixed by `tracing.WithMetricsOpts(...)`. If a tracer is created without that option, the wrapper registers the metric family under the tracing package. The concrete case in this catalog is `fsc_view_services_tracing_listens_*`: the tracer is declared in the Fabric committer package, but the exported family name is derived from `platform/view/services/tracing`.

### Node Export vs. Library Metrics

The node-facing families are exported by an FSC node when `fsc.metrics.provider = prometheus`. These include:

- all 29 explicit metric families
- all trace-derived families except `fsc_view_services_view_grpc_client_client_*` and `fsc_view_services_view_grpc_client_node_view_client_*`

The two caller-process families are exported only by the process that creates those tracers:

- `fsc_view_services_view_grpc_client_client_*`
- `fsc_view_services_view_grpc_client_node_view_client_*`

When validating the catalog from an FSC node's `/metrics` endpoint, those two caller-process families are expected to be absent unless the node process itself is the tracer owner.

### Operator Summary

For node-level monitoring, the quickest way to interpret the FSC metrics surface is:

- if the node is up and metrics are enabled, expect `fsc_view_services_metrics_operations_fsc_version` on `/metrics`
- if the node is serving views, expect `fsc_view_services_view_contexts`, `fsc_view_services_view_calls_*`, and one or both of `fsc_view_services_view_grpc_server_requests_*` and `fsc_view_services_view_web_view_client_*`, depending on whether callers use gRPC, the web endpoint, or both
- if the node uses libp2p, expect `fsc_view_services_comm_host_libp2p_bytes_sent` and `fsc_view_services_comm_host_libp2p_bytes_received`
- if the node uses WebSocket transport, expect `fsc_view_services_comm_host_websocket_ws_opened_websockets`, `fsc_view_services_comm_host_websocket_ws_opened_subconns`, and `fsc_view_services_comm_host_websocket_ws_active_subconns`
- if the node participates in Fabric transaction execution, expect the Fabric ordering, committer, delivery, vault, and finality-manager families listed in this catalog
- do not expect `fsc_view_services_view_grpc_client_client_*` or `fsc_view_services_view_grpc_client_node_view_client_*` on the node unless the node process itself is the tracer owner

If a family is absent, check the matching runtime condition first: communication transport, ingress path, Fabric execution path, or caller-process ownership.

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

Operational notes:

- `sessions` tracks the size of the session map, not a lifetime total.
- `stream_hashes` tracks distinct cache buckets, not individual streams.
- `opened_streams` is incremented only when FSC creates a fresh outbound stream because the cache could not be reused.
- `dropped_messages` is incremented in two main failure modes:
  - the dispatcher cannot deliver to a closed session
  - a session enqueue times out because the internal queue is full, which also closes the session

Primary implementation: `platform/view/services/comm/metrics.go`, with updates in `platform/view/services/comm/master.go`, `platform/view/services/comm/p2p.go`, and `platform/view/services/comm/session.go`.

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

### View gRPC Service

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_view_services_view_grpc_server_requests_received` | counter | `command` | Number of unary view-service requests that passed the initial validation and reached the processor dispatch stage. |
| `fsc_view_services_view_grpc_server_requests_completed` | counter | `command`, `success` | Number of unary view-service requests that completed processing. |

Operational notes:

- These counters are updated only on the unary `ProcessCommand` path.
- The streaming `StreamCommand` path is not currently counted by these families.
- `command` is the Go type of the command payload, for example `*protos.Command_CallView`.

Primary implementation: `platform/view/services/view/grpc/server/metrics.go`, updated in `platform/view/services/view/grpc/server/server.go`.

### gRPC Connection Metrics

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_view_services_grpc_conn_opened` | counter | none | Number of gRPC connections opened. |
| `fsc_view_services_grpc_conn_closed` | counter | none | Number of gRPC connections closed. |

These counters are implemented by FSC's custom gRPC server stats handler. The handler itself is tested, but the default runtime currently wires the gRPC server through OpenTelemetry's server handler. See [Current Caveats](#current-caveats).

Primary implementation: `platform/view/services/grpc/metrics.go`, with the custom stats handler in `platform/view/services/grpc/serverstatshandler.go`.

### Fabric Ordering

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `fsc_fabric_core_generic_ordering_ordered_transactions` | counter | `network` | Number of transactions that were successfully accepted by the ordering service. |

This counter is incremented only after the orderer returns `Status_SUCCESS`.

Primary implementation: `platform/fabric/core/generic/metrics/metrics.go`, updated in `platform/fabric/core/generic/ordering/cft.go`.

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
| `fsc_view_services_view_grpc_server_view_handler` | `fid`, `success` | View handler tracer used for the `initiate_view` path. |
| `fsc_view_services_view_grpc_client_client` | none | Outbound gRPC view invocation tracer. This family is emitted by the caller process, not by the remote FSC node that handles the request. |
| `fsc_view_services_view_grpc_client_node_view_client` | `fid` | Local client tracer used when a node invokes a view through the in-process local client helper. |
| `fsc_view_services_view_web_view_client` | `vid` | Web view client tracer used for REST or WebSocket view invocation. |

Each metric base above exports:

- `<metric-base>_operations`
- `<metric-base>_duration`

Primary implementation:

- `platform/view/services/view/context.go`
- `platform/view/services/view/web/view.go`
- `platform/view/services/view/grpc/client/client.go`
- `platform/view/services/view/grpc/client/local.go`
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
| `fsc_view_services_tracing_listens` | none | Committer listener tracer. The tracer is declared in the Fabric committer package but created without `tracing.WithMetricsOpts(...)`, so the exported family name is derived from `platform/view/services/tracing`. No span start was found on the current implementation path. |
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
| `command` | gRPC command payload type handled by the view service | `*protos.Command_CallView`, `*protos.Command_InitiateView` |
| `context_id` | FSC context identifier used by the WebSocket multiplexer tracer | FSC context UUID or caller-provided context ID |
| `fid` | View factory identifier used by gRPC-oriented view tracers | application-defined factory ID |
| `initiator_view` | Initiator view identifier associated with a child view execution | fully qualified view identifier or empty |
| `network` | Fabric network name | deployment-specific network name |
| `peer` | Remote libp2p peer identifier | peer ID string |
| `protocol` | Libp2p protocol identifier | protocol ID string |
| `side` | WebSocket connection side | `client`, `server` |
| `status` | Committer handler outcome classification | `not_found`, `failure`, `successful` |
| `success` | Success flag used by `fsc_view_services_view_calls_*`, `fsc_view_services_view_grpc_server_requests_completed`, `fsc_view_services_view_grpc_server_view_service_*`, and `fsc_view_services_view_grpc_server_view_handler_*` | `true`, `false`, or an empty value where the current implementation declares the label but does not populate it |
| `type` | Delivery message type classification | `unknown`, `block`, `status`, `other` |
| `version` | FSC version string | node version |
| `vid` | View identifier used by the web client tracer | application-defined view ID |
| `view` | Fully qualified FSC view identifier | Go package and type-based identifier |

## Current Caveats

The following families have implementation caveats:

- `fsc_view_services_view_grpc_server_view_service_*`: the tracer is declared, but no span start was found in the current code path, so these families are expected to remain idle.
- `fsc_view_services_view_grpc_server_view_handler_*`: the tracer is active for `initiate_view`, but the `success` label is declared without a matching attribute update. The exported `initiate_view` series therefore uses `success=""`.
- `fsc_view_services_tracing_listens_*`: this tracer is logically part of the Fabric committer, but because it is created without `tracing.WithMetricsOpts(...)`, the exported metric families are currently namespaced under `platform/view/services/tracing` rather than `platform/fabric/core/generic/committer`. 
- `fsc_view_services_comm_host_websocket_ws_closed_subconns`: the counter family is declared, but no increment site was found in the current production code.
- `fsc_view_services_grpc_conn_opened` and `fsc_view_services_grpc_conn_closed`: FSC's custom handler for these counters is tested, but the default runtime wiring currently uses OpenTelemetry's gRPC server handler instead of the FSC-specific one.
- `fsc_view_services_view_grpc_client_client_*`: these are caller-process metrics. The standard integration harness creates its external gRPC client with `tracing.NewProviderFromConfig(...)`, not with the metrics-wrapping tracing provider, so these families are not expected on the FSC node's `/metrics`.
- `fsc_view_services_view_grpc_client_node_view_client_*`: these are emitted only when the in-process local client helper is used inside the node. They are not part of the normal external gRPC or web ingress path.

