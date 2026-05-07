# FSC Node CLI Reference

## Overview

FSC nodes are Go applications that embed the Fabric Smart Client library. You create a custom node application that registers your views (business logic) and starts the FSC node.

## Creating a Node Application

FSC nodes require a Go application that embeds the FSC library:

```go
package main

import (
    "fmt"
    
    fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
    sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

func main() {
    node := fscnode.New()
    if err := node.InstallSDK(sdk.NewSDK(node)); err != nil {
        panic(err)
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

### `version`

Display version information.

**Usage:**
```bash
./mynode version
```

**Output:**
```
node:
 Go version: go1.21.0
 OS/Arch: darwin/arm64
```

**Description:**
- Shows program name
- Displays Go version used to build
- Shows OS and architecture

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
1. Stop accepting new requests
2. Complete in-flight operations
3. Close connections
4. Exit cleanly

**Note:** The signal handling uses Go's `signal.NotifyContext` for cross-platform compatibility. On Unix-like systems, both SIGINT and SIGTERM trigger graceful shutdown. On Windows, Ctrl+C and Ctrl+Break are handled appropriately.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Clean shutdown |
| `1` | Error during startup or operation |
