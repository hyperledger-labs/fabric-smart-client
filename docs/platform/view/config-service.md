# Configuration Service

## Overview

The configuration service in the View Platform is responsible for loading, managing, and providing access to application settings. 
It follows a hierarchical structure and supports loading configuration from multiple sources, including YAML files and environment variables.

## Key Features and Guarantees

* **Hierarchical Structure:** Configuration is organized in a nested tree structure using dot-separated keys (e.g., `fsc.id`, `fabric.network.name`).
* **Case Insensitivity:** Configuration keys are **case-insensitive**. Whether you use `FSC.ID`, `fsc.id`, or `Fsc.Id`, the service will correctly identify and retrieve the corresponding value.
* **Multi-source Loading:**

  * **YAML Files:** By default, the service looks for a `core.yaml` file in several locations (current directory, `/etc/hyperledger-labs/fabric-smart-client-node`, or the path specified by the `FSCNODE_CFG_PATH` environment variable).
  * **Environment Variables:** Configuration can be overridden or supplemented via environment variables prefixed with `CORE_`.
* **Type Safety:** The service provides methods to retrieve configuration values as specific types:
  * `GetBool(key)`: Returns a boolean.
  * `GetDuration(key)`: Returns a `time.Duration`.
  * `GetInt(key)`: Returns an integer.
  * `GetString(key)`: Returns a string.
  * `GetStringSlice(key)`: Returns a slice of strings.
* **Path Translation:** The `GetPath(key)` method provides path translation. If a configuration value represents a relative path, it is automatically converted to an absolute path relative to the configuration file's location.
* **Configuration Merging:** The service supports dynamically merging additional configuration from byte buffers (YAML format) at runtime.
* **Events:** It provides an event system to notify listeners when the configuration is merged or updated.

## Environment Variable Substitution

The configuration service automatically maps environment variables with the `CORE_` prefix to configuration keys. The mapping follows these rules:
1. The prefix `CORE_` is removed.
2. The remaining part is converted to lowercase.
3. Underscores (`_`) are replaced with dots (`.`) to represent nesting.

**Example:**
The environment variable `CORE_FSC_ID` maps to the configuration key `fsc.id`.
The environment variable `CORE_FABRIC_NETWORK1_NAME` maps to the configuration key `fabric.network1.name`.

*Note:* If the configuration key already exists with a specific type (e.g., integer or boolean), the environment variable value will be automatically converted to that type.

## Examples

### YAML Configuration (`core.yaml`)

```yaml
fsc:
  id: node1
  kvs:
    persistence:
      type: sql
      opts:
        driver: sqlite
        dataSource: ./db/kvs.db

logging:
  spec: info
  format: text

# Nested structure
fabric:
  network1:
    name: main_network
    enabled: true
```

### Accessing Configuration in Go

```go
func (s *MyService) DoSomething(config *config.Provider) {
    // Get node ID
    nodeID := config.ID() // equivalent to config.GetString("fsc.id")
    
    // Get a string
    logSpec := config.GetString("logging.spec")
    
    // Get a boolean
    enabled := config.GetBool("fabric.network1.enabled")
    
    // Get a path (automatically translated if relative)
    dbPath := config.GetPath("fsc.kvs.persistence.opts.dataSource")
    
    // Unmarshal a sub-tree into a struct
    var opts MyOptions
    err := config.UnmarshalKey("fabric.network1", &opts)
}
```

### Merging Configuration at Runtime

```go
func (s *MyService) UpdateConfig(config *config.Provider, rawYaml []byte) {
    err := config.MergeConfig(rawYaml)
    if err != nil {
        // handle error
    }
}
```

## Implementation Details

Currently, the configuration service is based on [github.com/knadh/koanf](https://github.com/knadh/koanf), a light-weight, extensible configuration management library for Go. While the `config.Provider` interface abstracts most of these details, the underlying implementation utilizes `koanf` for its powerful parsing, merging, and environment variable substitution capabilities.
