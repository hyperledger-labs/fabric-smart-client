# FSC node CLI reference

## Overview

FSC nodes are Go applications that embed the Fabric Smart Client library. You
create a custom node application that registers your views (business logic) and
starts the FSC node.

## Creating a node application

FSC nodes require a Go application that embeds the FSC library:

```go
package main

import (
    "fmt"
    "log"

    fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
    sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

func main() {
    node := fscnode.New()
    if err := node.InstallSDK(sdk.NewSDK(node)); err != nil {
        log.Fatalf("failed installing SDK: %v", err)
    }
    node.Execute(func() error {
        fmt.Println("Hello World")
        return nil
    })
}
```

Build your node:

```bash
go build -o mynode .
```

## Commands

### `node start`

Start the FSC node.

**Usage:**

```bash
./mynode node start
```

**Description:**

Starts the FSC node that:

- Loads configuration from `core.yaml`
- Initializes the View SDK
- Handles graceful shutdown on signals

**Configuration Required:**

The node requires a `core.yaml` file in:

- Current directory, OR
- Directory specified by `FSCNODE_CFG_PATH` environment variable

See [Configuration Guide](../configuration.md) for complete configuration
options.

**Examples:**

1. **Start with default configuration:**

   ```bash
   # Looks for ./core.yaml
   ./mynode node start
   ```

2. **Start with custom configuration path:**

   ```bash
   export FSCNODE_CFG_PATH=/etc/fsc
   ./mynode node start
   ```

3. **Start with profiling enabled:**

   ```bash
   export FSCNODE_PROFILER=true
   ./mynode node start
   ```

4. **Start in background:**

   ```bash
   nohup ./mynode node start > node.log 2>&1 &
   ```

### `node migrate`

Prepare the schema of every store the node uses and exit.

**Usage:**

```bash
./mynode node migrate             # run the pending schema steps up to the first contract step
./mynode node migrate --check     # run nothing; exit 10 and list the pending steps if there are any
./mynode node migrate --contract  # also run contract steps, once no older binary opens the store
./mynode node migrate --contract --forget-opener node1  # first delete node1's rows in the openers table
```

The command reads the same configuration as `node start` and installs every SDK,
which loads the node's identities, but starts no server, P2P or delivery. It
then opens each store the SDKs register (see
[Reaching stores from `node migrate`](../platform/view/services/db-driver.md#reaching-stores-from-node-migrate)),
with `skipCreateTable` set to `false` for this run, or to `true` with `--check`.
It opens every store even if one fails, and reports each failing store. With
`--check` it does so even when installing an SDK fails on a store, so a store
may be listed twice. Before connecting to Postgres it logs the host, database
and role of each persistence, never the password. A store that would fall back
to memory, because it names no persistence and `fsc.persistences.default` is not
defined, fails the command. Run it before starting nodes that use
`skipCreateTable: true`; see
[Schema migrations](../platform/view/services/schema-migrations.md).

Without `--contract` the command, like a starting node, stops before a contract
step and does not count it as pending. With `--contract` it runs contract steps.
`--check --contract` counts them as pending, marks them `(contract)`, and exits
12 if a binary that lacks one opens the store. `--forget-opener <fsc.id>`,
repeatable, deletes that node's rows from each store's openers table before any
step runs; it cannot be combined with `--check`. See
[Contract steps](../platform/view/services/schema-migrations.md#contract-steps).

| Exit code | Meaning                                                                                                                                              |
| --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `0`       | every store is current (or, without `--check`, has been migrated)                                                                                    |
| `10`      | `--check`: steps are pending; each pending store and its steps are listed                                                                            |
| `11`      | a store's schema is newer than this binary (`migrate.ErrSchemaTooNew`)                                                                               |
| `12`      | `--contract`, also with `--check`: a store is opened by a binary that lacks a pending contract step (`migrate.ErrOlderOpener`); the nodes are listed |
| `1`       | the command could not run or could not check or migrate a store: configuration, connection, lock or step failure                                     |
| `2`       | invalid arguments; no store was opened                                                                                                               |

When stores fail for different reasons, the code is the first of `1`, `11`,
`12`, `10` that applies. An unknown flag or argument exits 2, and `--help`
connects to nothing.

### `node peers --check`

Check, before a rollout, that this binary can still talk to the node's peers.

**Usage:**

```bash
./mynode node peers --check
```

The command reads the node's configuration, as `node start` does, but installs
no SDK and opens no database. It refuses the resolver entries the node refuses,
and like the node it uses only the first entry of each name; later entries of a
name are bound to it and never dialed. For each such entry that has a `P2P`
address, other than the node itself, it dials the peer over WebSocket P2P with
the node's identity and TLS settings, checks that the peer's TLS certificate
belongs to the resolver's identity, reads the
[preamble](../platform/view/services/comm/readme.md) the peer sends on the
upgrade response, and disconnects. It opens no session and starts no view, so
the peer runs nothing. Each peer gets 10 seconds.

For each peer it prints the peer's FSC version and framing epoch and, per
component, both ranges and the version they use, the highest one in both ranges.
A component the peer does not announce counts as version 1, and so does every
component of a peer of a release without preambles, as in a session: the check
fails if version 1 is outside this binary's range, such as for `fabric` and
`fabricx`. Run the new binary with the node's configuration and key material.
Only `fsc.p2p.type: websocket` is supported; with libp2p the command exits 1.

| Exit code | Meaning                                                                                         |
| --------- | ----------------------------------------------------------------------------------------------- |
| `0`       | every peer is compatible                                                                        |
| `11`      | a peer's framing epoch differs, or a component's ranges do not overlap                          |
| `1`       | a peer could not be reached, or the command could not run: configuration, identity or transport |
| `2`       | invalid arguments, including a missing `--check`; nothing was dialed                            |

When peers fail for different reasons, `1` wins over `11`.

### `version`

Display version information.

**Usage:**

```bash
./mynode version
```

**Output:**

```text
node:
 Go version: go1.27.3
 OS/Arch: darwin/arm64
 FSC version: v0.28.0
 Application: github.com/acme/app v1.4.0
 Revision: 7c647bd239b9e626ec4f02d4a2cca157c251e264 (modified)
```

With `--json` it prints one JSON object for release tooling:

```json
{
  "fsc_version": "v0.28.0",
  "app_module": "github.com/acme/app",
  "app_version": "v1.4.0",
  "vcs_revision": "0123abcd",
  "vcs_modified": false,
  "framing_epoch": 1,
  "components": [{"name": "view", "min": 1, "max": 1}, {"name": "fabric", "min": 1, "max": 2}, {"name": "fabricx", "min": 1, "max": 2}],
  "stores": [{"type": "vault", "dialect": "postgres", "steps": [{"version": 1, "name": "create tables", "hash": "a9dffb69…", "contract": false}]}]
}
```

**Description:**

- Shows program name, the Go version used to build, and the OS and architecture
- `FSC version` is the Fabric Smart Client module the binary was built with;
  with a `replace` in `go.mod` it is the replacement's version, or `(devel)` for
  a local directory
- `Application` is the main module and its version; a build from a local
  checkout prints a pseudo-version or `(devel)`, and a binary without build info
  prints `unknown`
- `Revision` is the main module's VCS revision, with `(modified)` for a dirty
  working tree, and `unknown` when the build recorded no VCS information (e.g.
  `go build -buildvcs=false`)
- `--json` prints the same build info, empty instead of `unknown`, with
  `vcs_modified` for a dirty tree; `framing_epoch` and `components` are what the
  binary sends in its stream preamble: FSC's components and those the
  application registers with `preamble.RegisterComponent`; `stores` lists, per
  SQL dialect, every store type the binary's SDKs register when they are built,
  with its schema steps, `migrate.Step.Hash` and `contract`, true for a contract
  step: once it ran, binaries without it refuse the store, so rolling back past
  it is a restore (the memory driver uses the `sqlite` steps). The view SDK
  registers the view stores and the Fabric SDK the Fabric stores, so a binary
  without the Fabric SDK lists only the view stores; see
  [Reporting store schemas](../platform/view/services/db-driver.md#reporting-store-schemas).
  Hashes are those of a node without a table prefix, since the step SQL names
  the tables
- The field names are a contract for release tooling: fields are added, never
  renamed or removed
- `version` reads no configuration and opens no database, so it runs on a build
  machine without `core.yaml`. Without one, `node migrate` and
  `node peers --check` exit 1 with the missing-file error, and `node start`
  fails with it like any other start error

## Environment variables

All environment variables use the `FSCNODE_` prefix.

### `FSCNODE_CFG_PATH`

**Description:** Path to directory containing `core.yaml`

**Type:** String (directory path)

**Default:** `./` (current directory)

**Example:**

```bash
export FSCNODE_CFG_PATH=/etc/fsc
./mynode node start
```

### `FSCNODE_PROFILER`

**Description:** Enable Go profiling for performance analysis

**Type:** Boolean (`true` or `false`)

**Default:** `false`

**Example:**

```bash
export FSCNODE_PROFILER=true
./mynode node start
```

When enabled, the following profiling data files are written to the
configuration directory:

- `cpu.pprof` - CPU profiling data
- `mem-heap.pprof` - Memory heap profiling
- `mem-allocs.pprof` - Memory allocations profiling
- `mutex.pprof` - Mutex contention profiling
- `block.pprof` - Blocking profiling

These files can be analyzed using Go's pprof tool, for example:

```bash
go tool pprof cpu.pprof
```

## Signal handling

The FSC node handles these signals for graceful shutdown:

| Signal            | Behavior          |
| ----------------- | ----------------- |
| `SIGINT` (Ctrl+C) | Graceful shutdown |
| `SIGTERM`         | Graceful shutdown |

**Graceful shutdown:**

1. Stop the servers one after another (web, metrics, gRPC). Each stops accepting
   new requests and waits for its in-flight ones:
   - web and metrics: requests get 5s to finish, then their contexts are
     cancelled and the server waits up to 10s more for its handlers to return
   - gRPC: requests are cancelled at once, and the server waits up to 10s for
     its handlers to return
2. Close storage, once those handlers have returned or the waits have timed out
3. Exit cleanly

Views that observe their context (`ctx.Done()`) return promptly once it is
cancelled. A view that ignores it can still find storage closed if it outlives
the wait.

> [!NOTE]
> The node listens for both signals with Go's `signal.Notify`. A second SIGINT
> or SIGTERM during the shutdown skips the rest of the drain. On Windows, Ctrl+C
> and Ctrl+Break arrive as `os.Interrupt` and trigger the same shutdown.

## Exit codes

| Code                  | Meaning                                                                                                                      |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `0`                   | Clean shutdown                                                                                                               |
| `1`                   | Error during startup or operation                                                                                            |
| `2`, `10`, `11`, `12` | `node migrate` and `node peers --check`; see [`node migrate`](#node-migrate) and [`node peers --check`](#node-peers---check) |
