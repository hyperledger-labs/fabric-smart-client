# Node Operations Guide

## Overview

This guide provides practical usage patterns for starting and operating a Fabric Smart Client (FSC) node.
It complements the [configuration reference](../configuration.md) by focusing on the operational tasks an operator performs most often:

- starting the node
- choosing a configuration location
- overriding configuration with environment variables
- exposing gRPC, web, P2P, and metrics endpoints
- securing inbound connections
- selecting persistence options
- handling shutdown and common startup failures

Throughout this guide, `<node-binary>` means the executable that embeds the FSC node package.
The exact filename depends on the application, but the standard FSC node command exposed by the package is:

```bash
<node-binary> node start
```

This guide does not cover `fsccli`, which is a separate developer-oriented utility with a different command tree.

## What Happens on Startup

When an FSC-based application runs `node start`, the standard node package performs the following steps:

1. Loads `core.yaml` using the configuration service.
2. Installs the configured SDKs.
3. Starts the SDKs.
4. Runs post-start hooks for SDKs that expose them.
5. Starts the configured listeners and blocks until the process receives a termination signal.

For operators, the important point is that the node is mostly configured through `core.yaml` and environment variables, not through a large set of CLI flags.

## Before You Start

Before starting an FSC node, make sure you have:

- a `core.yaml` file for the application
- the certificate and key files referenced by the configuration
- the required listen ports available for the enabled services
- a persistence configuration appropriate for the environment

If the configuration uses relative paths, FSC resolves them relative to the directory that contains `core.yaml`.
This makes it practical to keep certificates, keys, and local databases next to the configuration file.

## Starting the Node

### Start from the current directory

If `core.yaml` is in the current working directory, start the node as follows:

```bash
cd /path/to/node-config
<node-binary> node start
```

This is the simplest pattern for local development and integration environments.

### Start from an explicit configuration directory

The primary operator mechanism for choosing a configuration directory is `FSCNODE_CFG_PATH`:

```bash
export FSCNODE_CFG_PATH=/etc/hyperledger-labs/fabric-smart-client-node
<node-binary> node start
```

Use this pattern when the process is started by a service manager or when the working directory is not stable.

### Enable runtime profiling

To collect profiling artifacts during startup and runtime:

```bash
export FSCNODE_CFG_PATH=/var/lib/fsc/node1
export FSCNODE_PROFILER=true
<node-binary> node start
```

When profiling is enabled, the node writes `*.pprof` files under `FSCNODE_CFG_PATH` if it is set, or under the current working directory otherwise.
This is useful when diagnosing CPU, heap, allocation, mutex, or blocking issues.

### Ignore `SIGHUP`

By default, the node exits on `SIGHUP`.
To keep the process alive when the controlling terminal disappears:

```bash
export FSCNODE_SIGHUP_IGNORE=true
<node-binary> node start
```

This is useful in terminal-driven environments.
In managed environments, prefer using a process supervisor instead of relying on `SIGHUP` handling.

### Combined example

```bash
export FSCNODE_CFG_PATH=/etc/hyperledger-labs/fabric-smart-client-node
export FSCNODE_PROFILER=false
export FSCNODE_SIGHUP_IGNORE=true
export CORE_LOGGING_SPEC=info

<node-binary> node start
```

## Configuration Discovery and Overrides

### Configuration search order

The standard configuration provider looks for `core.yaml` in the following order:

1. A path supplied programmatically by the application, if it uses one.
2. `FSCNODE_CFG_PATH`, if set.
3. The current working directory.
4. `/etc/hyperledger-labs/fabric-smart-client-node`.

When `FSCNODE_CFG_PATH` is set, it becomes the only path considered by the default loader.

### Overriding configuration with `CORE_` variables

The node startup wrapper uses `FSCNODE_*` variables for process-level behavior.
The configuration service uses `CORE_*` variables to override values inside `core.yaml`.

Examples:

```bash
export CORE_FSC_ID=node1
export CORE_LOGGING_SPEC=info
export CORE_FSC_GRPC_ENABLED=true
export CORE_FSC_GRPC_ADDRESS=0.0.0.0:20000
export CORE_FSC_WEB_ENABLED=true
export CORE_FSC_WEB_ADDRESS=0.0.0.0:20002
export CORE_FSC_METRICS_PROVIDER=prometheus
```

The mapping rules are:

1. Remove the `CORE_` prefix.
2. Convert the remaining name to lowercase.
3. Replace `_` with `.`.

For example, `CORE_FSC_ID` maps to `fsc.id`.

If the target key already exists as a boolean, integer, or float, FSC preserves that type when the environment variable is loaded.

## Main Services and Ports

The default FSC node wiring can expose several listeners, depending on the configuration.

| Service | Main configuration keys | Purpose | Notes |
| --- | --- | --- | --- |
| P2P transport | `fsc.p2p.type`, `fsc.p2p.listenAddress` | Node-to-node FSC traffic | Transport-specific security depends on `libp2p` or `websocket`. |
| gRPC server | `fsc.grpc.*` | Programmatic access to FSC services | Supports TLS and optional mutual TLS. |
| Web server | `fsc.web.*` | REST view calls and operations endpoints | The default wiring serves both application and operations paths on the same listener. |
| Metrics exporter | `fsc.metrics.*` | Prometheus scrape endpoint | Registered on the web server at `/metrics`. |
| Runtime log control | `fsc.web.*` | Read and update the active log spec | Registered on the web server at `/logspec`. |

### Important operational detail

The operations endpoints do not run on a dedicated listener in the default node wiring.
They are attached to the same FSC web server used by the REST view handler.

If `fsc.web.enabled` is `false`, the node does not open a real HTTP listener, and `/`, `/metrics`, and `/logspec` are not externally reachable.

## Endpoint Behavior and Security

### gRPC server

The gRPC server is controlled by `fsc.grpc.enabled` and `fsc.grpc.address`.
TLS is controlled under `fsc.grpc.tls`.

Typical secure configuration:

```yaml
fsc:
  grpc:
    enabled: true
    address: 0.0.0.0:20000
    tls:
      enabled: true
      clientAuthRequired: true
      cert:
        file: ./tls/server.crt
      key:
        file: ./tls/server.key
      clientRootCAs:
        files:
          - ./tls/clients-ca.crt
```

Operational guidance:

- Enable TLS for any non-local deployment.
- Use `clientAuthRequired: true` when the server should only accept known clients.
- Configure `keepalive` if you expect long-lived clients or need stricter connection rotation.

### Web server

The web server is controlled by `fsc.web.enabled` and `fsc.web.address`.
TLS is controlled under `fsc.web.tls`.

The default node wiring registers the root handler `/` as a secure handler.
In practice, that means requests must arrive with a verified client certificate.

Operational guidance:

- If you intend to use the REST view endpoint, configure web TLS together with a client CA pool.
- For non-local deployments, prefer `clientAuthRequired: true`.
- If web TLS is enabled but client certificate verification is not configured correctly, the listener can start successfully while secure handlers still reject requests.

### `/logspec`

The operations system registers `/logspec` on the web server.
This endpoint allows operators to inspect or update the active logging specification at runtime.

When web TLS is enabled, `/logspec` is registered as a secure handler.
In practice, this means operators should configure client certificate verification if they plan to use it.

### `/metrics`

Metrics are exposed only when `fsc.metrics.provider = prometheus`.
The metrics endpoint is served at `/metrics` on the FSC web server.

The handler security is controlled by `fsc.metrics.prometheus.tls`:

- If `false`, the handler does not require a client certificate.
- If `true`, the handler is registered as a secure handler and therefore requires a verified client certificate.

If the web server itself uses TLS, the metrics endpoint is still served over HTTPS even when `fsc.metrics.prometheus.tls` is `false`.

## Choosing a P2P Transport

FSC supports multiple P2P transports.
Operators should choose the transport based on network topology and security requirements.

### Libp2p

Use `libp2p` when:

- the deployment is natively peer-to-peer
- custom P2P ports are acceptable
- peer discovery and decentralized networking behavior are expected

This is the default transport in the configuration reference.

### WebSocket

Use `websocket` when:

- the deployment is behind restrictive firewalls
- standard HTTP(S) ports are preferred
- the environment already manages HTTPS certificates and reverse-proxy-like connectivity

For WebSocket transport, FSC enforces mutual TLS as the transport security model.
For full details, see the [WebSocket transport guide](../platform/view/services/comm/websocket.md).

## Persistence Choices

Persistence is configured under `persistences`.
From an operator point of view, the main decision is whether the node should be ephemeral or durable.

### `memory`

Use `memory` for:

- tests
- short-lived development runs
- disposable integration environments

Do not use it for long-lived nodes whose state must survive restarts.

### `sqlite`

Use `sqlite` when:

- the node runs as a single process on one host
- local disk persistence is sufficient
- a simple operational model is preferred

This is a good default choice for many development and small deployment setups.

### `postgres`

Use `postgres` when:

- the deployment already relies on managed database infrastructure
- backup, monitoring, and DB-level operational controls are required
- the node should use an external durable store instead of local disk files

## Recommended Operating Patterns

### Local development node

Use:

- `FSCNODE_CFG_PATH` pointing to a local workspace
- `sqlite` persistence
- loopback listen addresses where possible
- `prometheus` metrics if you want quick inspection

Example:

```bash
export FSCNODE_CFG_PATH=$HOME/fsc/node1
export CORE_LOGGING_SPEC=debug
export CORE_FSC_METRICS_PROVIDER=prometheus

<node-binary> node start
```

### Service-managed node

Use:

- `/etc/hyperledger-labs/fabric-smart-client-node` or another stable configuration directory
- absolute certificate paths or paths relative to that directory
- durable persistence
- explicit TLS configuration for gRPC and web listeners

This is the preferred pattern for long-running nodes started by a supervisor.

### Firewall-friendly node

Use:

- `fsc.p2p.type: websocket`
- properly provisioned TLS materials for both server and client verification
- standard HTTPS-accessible endpoints

This pattern is useful when direct libp2p-style connectivity is not practical.

### Monitored node

Use:

- `fsc.web.enabled: true`
- `fsc.metrics.provider: prometheus`
- `fsc.metrics.prometheus.tls: true` if scrapes should require client certificates

This allows Prometheus-based monitoring without introducing a separate exporter process.

## Observability

### Logs

The main log controls live under `logging`.
The most important operator setting is usually `logging.spec`.

You can set it in `core.yaml`:

```yaml
logging:
  spec: info
```

Or override it at startup:

```bash
export CORE_LOGGING_SPEC=debug
```

If the web server is enabled, `/logspec` can be used to inspect or update the logging spec at runtime.

### Metrics

To expose Prometheus metrics:

```yaml
fsc:
  web:
    enabled: true
    address: 0.0.0.0:20002
  metrics:
    provider: prometheus
    prometheus:
      tls: false
```

With this configuration, Prometheus can scrape:

```text
http://<host>:20002/metrics
```

If web TLS is enabled, use `https://<host>:20002/metrics` instead.

### Traces

Tracing is configured under `fsc.tracing`.
For operational use, the main decision is the provider:

- `none` for no tracing
- `console` for local inspection
- `file` for local artifact collection
- `otlp` for external collectors

For more details, see the [monitoring documentation](../platform/view/services/monitoring.md).

### Profiling

If `FSCNODE_PROFILER=true`, the node writes profiling artifacts such as:

- `cpu.pprof`
- `mem-heap.pprof`
- `mem-allocs.pprof`
- `mutex.pprof`
- `block.pprof`

These files are written under `FSCNODE_CFG_PATH` if it is set, or under the current working directory otherwise.

## Shutdown and Restart Behavior

The standard startup wrapper handles the following signals:

- `SIGINT`
- `SIGTERM`
- `SIGHUP`

By default, `SIGHUP` stops the node.
If `FSCNODE_SIGHUP_IGNORE=true`, the node logs the event and continues running.

On shutdown, the node cancels its main context and stops the configured services, including the web server, gRPC server, KVS, and operations system.

Operational guidance:

- Prefer durable persistence for restartable nodes.
- Use stable configuration directories so relative paths remain valid after restart.
- Treat most startup configuration changes as restart-requiring changes.

## Troubleshooting

### `Could not find config file`

If startup fails with a configuration lookup error:

- verify that `core.yaml` exists
- make sure `FSCNODE_CFG_PATH` points to a directory, not to the file itself
- confirm that the process can read the directory

### Listener fails to bind

If the node cannot bind a port:

- check whether another process is already using the address
- verify that the configured address format matches the service type
- confirm that the service is actually enabled before expecting it to listen

### TLS files cannot be loaded

If startup fails while loading certificates or keys:

- verify that the referenced files exist
- remember that relative paths are resolved from the `core.yaml` directory
- make sure certificate, key, and CA files match the enabled TLS mode

### Web endpoint returns `401 Unauthorized`

If `/` or `/logspec` returns `401`:

- verify that the client is presenting a certificate
- verify that the certificate chains back to a configured client CA
- prefer `clientAuthRequired: true` for predictable mutual TLS behavior

### `/metrics` is not reachable

If metrics are missing:

- verify that `fsc.web.enabled` is `true`
- verify that `fsc.metrics.provider` is set to `prometheus`
- check whether web TLS requires HTTPS instead of HTTP
- if `fsc.metrics.prometheus.tls` is `true`, make sure the scraper presents a trusted client certificate

### State disappears after restart

If local state is lost after a restart:

- check whether the selected persistence type is `memory`
- switch to `sqlite` or `postgres` for durable environments

### Command name confusion

FSC application binaries embed the node package, but the binary name itself depends on the application.
The stable part for operators is the subcommand structure:

```bash
<node-binary> node start
```

## Code References

The following files back the operational behavior described in this guide:

| Topic | File |
| --- | --- |
| CLI wiring | `node/node.go` |
| Node start command | `node/start/start.go` |
| Node lifecycle | `pkg/node/node.go` |
| Config loading and `CORE_` overrides | `platform/view/services/config/provider.go` |
| Web, gRPC, and operations startup | `platform/view/sdk/dig/servers.go` |
| Metrics and runtime log endpoint registration | `platform/view/services/metrics/operations/system.go` |
| Web TLS and secure handler behavior | `platform/view/services/web/server/server.go` |
| WebSocket transport | `platform/view/services/comm/host/websocket/*` |
