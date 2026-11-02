# FSC Node CLI Reference

## Overview

FSC nodes are Go applications that embed the Fabric Smart Client library. You create a custom node application that registers your views (business logic) and starts the FSC node.

## Creating a Node Application

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

See [Configuration Guide](../configuration.md) for complete configuration options.

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
./mynode node migrate          # run every pending schema step
./mynode node migrate --check  # run nothing; exit 10 and list the pending steps if there are any
```

The command reads the same configuration as `node start` and installs every SDK, which loads the node's identities, but starts no server, P2P or delivery. It then opens each store the SDKs register (see [Reaching stores from `node migrate`](../platform/view/services/db-driver.md#reaching-stores-from-node-migrate)), with `skipCreateTable` set to `false` for this run, or to `true` with `--check`. It opens every store even if one fails, and reports each failing store. With `--check` it does so even when installing an SDK fails on a store, so a store may be listed twice. Before connecting to Postgres it logs the host, database and role of each persistence, never the password. A store that would fall back to memory, because it names no persistence and `fsc.persistences.default` is not defined, fails the command. Run it before starting nodes that use `skipCreateTable: true`; see [Schema migrations](../platform/view/services/schema-migrations.md).

| Exit code | Meaning |
|---|---|
| `0` | every store is current (or, without `--check`, has been migrated) |
| `10` | `--check`: steps are pending; each pending store and its steps are listed |
| `11` | a store's schema is newer than this binary (`migrate.ErrSchemaTooNew`) |
| `1` | the command could not run or could not check or migrate a store: configuration, connection, lock or step failure |
| `2` | invalid arguments; no store was opened |

When stores fail for different reasons, the code is the first of `1`, `11`, `10` that applies. Any argument other than `--check` and `--help` exits 2, and `--help` connects to nothing.

### `version`

Display version information.

**Usage:**
```bash
./mynode version
```

**Output:**
```
node:
 Go version: go1.27.3
 OS/Arch: darwin/arm64
 FSC version: v0.27.0
 Application: github.com/acme/app v1.4.0
 Revision: 7c647bd239b9e626ec4f02d4a2cca157c251e264 (modified)
```

**Description:**
- Shows program name, the Go version used to build, and the OS and architecture
- `FSC version` is the Fabric Smart Client module the binary was built with; with a `replace`
  in `go.mod` it is the replacement's version, or `(devel)` for a local directory
- `Application` is the main module and its version; a build from a local checkout prints a
  pseudo-version or `(devel)`, and a binary without build info prints `unknown`
- `Revision` is the main module's VCS revision, with `(modified)` for a dirty working tree,
  and `unknown` when the build recorded no VCS information (e.g. `go build -buildvcs=false`)

## Environment Variables

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

When enabled, the following profiling data files are written to the configuration directory:
- `cpu.pprof` - CPU profiling data
- `mem-heap.pprof` - Memory heap profiling
- `mem-allocs.pprof` - Memory allocations profiling
- `mutex.pprof` - Mutex contention profiling
- `block.pprof` - Blocking profiling

These files can be analyzed using Go's pprof tool, for example:
```bash
go tool pprof cpu.pprof
```

## Signal Handling

The FSC node handles these signals for graceful shutdown:

| Signal | Behavior |
|--------|----------|
| `SIGINT` (Ctrl+C) | Graceful shutdown |
| `SIGTERM` | Graceful shutdown |

**Graceful shutdown:**
1. Stop the servers one after another (web, metrics, gRPC). Each stops accepting new requests and waits for its in-flight ones:
   - web and metrics: requests get 5s to finish, then their contexts are cancelled and the server waits up to 10s more for its handlers to return
   - gRPC: requests are cancelled at once, and the server waits up to 10s for its handlers to return
2. Close storage, once those handlers have returned or the waits have timed out
3. Exit cleanly

Views that observe their context (`ctx.Done()`) return promptly once it is cancelled. A view that ignores it can still find storage closed if it outlives the wait.

**Note:** The signal handling uses Go's `signal.NotifyContext` for cross-platform compatibility. On Unix-like systems, both SIGINT and SIGTERM trigger graceful shutdown. On Windows, Ctrl+C and Ctrl+Break are handled appropriately.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Clean shutdown |
| `1` | Error during startup or operation |
| `2`, `10`, `11` | `node migrate` only; see [`node migrate`](#node-migrate) |
