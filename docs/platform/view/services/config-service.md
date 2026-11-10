# Configuration Service

## Overview

The configuration service in the View Platform is responsible for loading, managing, and providing access to application settings. 
It follows a hierarchical structure and supports loading configuration from multiple sources, including YAML files and environment variables.

## Key Features and Guarantees

* **Hierarchical Structure:** Configuration is organized in a nested tree structure using dot-separated keys (e.g., `fsc.id`, `fabric.network.name`).
* **Case Insensitivity:** Configuration keys are **case-insensitive**. Whether you use `FSC.ID`, `fsc.id`, or `Fsc.Id`, the service will correctly identify and retrieve the corresponding value. Keys are lowercased on load, so a configuration with two keys at the same level that differ only in case, such as `Foo` and `foo`, is rejected. Likewise, a dotted key that names the same setting or section as a nested one, such as `fsc.id` next to `fsc: {id: ...}`, is rejected, also when the two come from different sources: the file and a `CORE_` environment variable, or the loaded configuration and one merged at runtime.
* **Multi-source Loading:**

  * **YAML Files:** By default, the service looks for a `core.yaml` file in several locations (current directory, `/etc/hyperledger-labs/fabric-smart-client-node`, or the path specified by the `FSCNODE_CFG_PATH` environment variable). The first location that contains a `core.yaml` is used; if that file cannot be opened, read or parsed (including a configured path that is a file rather than a directory), loading fails instead of moving on to the next location.
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
3. A double underscore (`__`) becomes a literal `_`, and every other underscore (`_`) becomes a dot (`.`) to represent nesting.

**Example:**
The environment variable `CORE_FSC_ID` maps to the configuration key `fsc.id`.
The environment variable `CORE_FABRIC_MYNETWORK_NAME` maps to the configuration key `fabric.mynetwork.name`.
The environment variable `CORE_FSC_PERSISTENCES_MY__DB_OPTS_DATASOURCE` maps to the configuration key `fsc.persistences.my_db.opts.datasource`.

*Note:* If the configuration key already exists with a specific type (e.g., integer or boolean), the environment variable value will be automatically converted to that type.

## Examples

### YAML Configuration (`core.yaml`)

```yaml
fsc:
  id: node1
  persistences:
    mydb:
      type: sqlite
      opts:
        dataSource: ./db/kvs.db
  kvs:
    persistence: mydb

logging:
  spec: info
  format: text

# Nested structure
fabric:
  mynetwork:
    name: main_network
    enabled: true
```

### Accessing Configuration in Go

```go
func (s *MyService) DoSomething(cp *config.Provider) {
    // Get node ID
    nodeID := cp.ID() // equivalent to cp.GetString("fsc.id")
    
    // Get a string
    logSpec := cp.GetString("logging.spec")
    
    // Get a boolean
    enabled := cp.GetBool("fabric.mynetwork.enabled")
    
    // Get a path (automatically translated if relative)
    dbPath := cp.GetPath("fsc.persistences.mydb.opts.dataSource")
    
    // Unmarshal a sub-tree into a struct
    var opts MyOptions
    err := cp.UnmarshalKey("fabric.mynetwork", &opts)
}
```

### Merging Configuration at Runtime

```go
func (s *MyService) UpdateConfig(cp *config.Provider, rawYaml []byte) {
    err := cp.MergeConfig(rawYaml)
    if err != nil {
        // handle error
    }
}
```

## Implementation Details

The configuration service is based on [github.com/knadh/koanf](https://github.com/knadh/koanf), a configuration library for Go. The `config.Provider` interface hides most of it; the implementation uses `koanf` for parsing, merging and environment variable substitution.
This implementation supports the field tag `yaml` as well. This allows the developer to support something like this:

```go
type Opts struct {
	// Source has on purpose a different name from what we expect in the configuration file as signaled by the yaml tag.
	Source       string `yaml:"dataSource"`
	SkipPragmas  bool
	MaxOpenConns int
}
```