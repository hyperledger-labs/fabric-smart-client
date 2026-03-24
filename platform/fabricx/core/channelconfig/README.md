# Channel Configuration Monitor Service

## Overview

The Channel Configuration Monitor Service automatically detects and applies channel configuration changes in Hyperledger Fabric networks. It periodically polls for new configuration transactions and updates the membership and ordering services when changes are detected.

## Features

- **Automatic Configuration Detection**: Periodically checks for new channel configuration transactions
- **Version Tracking**: Tracks configuration versions to avoid reprocessing
- **Automatic Updates**: Updates membership and ordering services when configuration changes
- **Retry Logic**: Exponential backoff retry mechanism for transient failures
- **Configurable Polling**: Adjustable polling interval and retry parameters
- **Lifecycle Management**: Explicit Start/Stop API for fine-grained control
- **Thread-Safe**: Safe for concurrent use

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                  ChannelConfigMonitor                        │
├─────────────────────────────────────────────────────────────┤
│  - Polling Loop (configurable interval)                      │
│  - Configuration Version Tracking (in-memory)                │
│  - Error Handling with Exponential Backoff                   │
│  - Lifecycle Management (Start/Stop API)                     │
└─────────────────────────────────────────────────────────────┘
           │                    │                    │
           ▼                    ▼                    ▼
    ┌──────────┐        ┌──────────┐        ┌──────────┐
    │  Query   │        │Membership│        │ Ordering │
    │ Service  │        │ Service  │        │ Service  │
    └──────────┘        └──────────┘        └──────────┘
```

## Usage

### Basic Usage

```go
import (
    "context"
    "time"
    
    "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channelconfig"
)

// Create configuration
config := &channelconfig.Config{
    PollInterval:      1 * time.Minute,
    MaxRetries:        5,
    InitialRetryDelay: 1 * time.Second,
    MaxRetryDelay:     5 * time.Minute,
}

// Create monitor
monitor, err := channelconfig.NewChannelConfigMonitor(
    config,
    queryService,      // Your query service implementation
    membershipService, // Your membership service implementation
    orderingService,   // Your ordering service implementation
    configService,     // Your config service implementation
    "mynetwork",       // Network name
    "mychannel",       // Channel name
)
if err != nil {
    // Handle error
}

// Start monitoring
ctx := context.Background()
if err := monitor.Start(ctx); err != nil {
    // Handle error
}

// Monitor runs in background...

// Stop monitoring when done
if err := monitor.Stop(); err != nil {
    // Handle error
}
```

### Configuration from ConfigService

```go
// Load configuration from config service
config, err := channelconfig.NewConfig(configService, "mynetwork", "mychannel")
if err != nil {
    // Handle error
}

// Create monitor with loaded config
monitor, err := channelconfig.NewChannelConfigMonitor(
    config,
    queryService,
    membershipService,
    orderingService,
    configService,
    "mynetwork",
    "mychannel",
)
```

## Configuration

Configuration can be provided via YAML configuration file:

```yaml
fabric:
  mynetwork:
    channels:
      mychannel:
        configmonitor:
          pollInterval: 60s        # How often to check for config changes
          maxRetries: 5            # Maximum retry attempts on failure
          initialRetryDelay: 1s    # Initial delay before first retry
          maxRetryDelay: 5m        # Maximum delay between retries
```

### Configuration Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `pollInterval` | duration | 1m | Time between configuration checks |
| `maxRetries` | int | 5 | Maximum number of retry attempts |
| `initialRetryDelay` | duration | 1s | Initial delay before first retry |
| `maxRetryDelay` | duration | 5m | Maximum delay between retries |

## How It Works

1. **Polling Loop**: The service runs a background goroutine that polls at the configured interval
2. **Version Check**: On each poll, it queries for the latest configuration transaction and compares versions
3. **Update Detection**: If a new version is detected, the service proceeds with updates
4. **Membership Update**: Calls `MembershipService.Update()` with the new configuration envelope
5. **Orderer Config Extraction**: Extracts orderer configuration from the membership service
6. **Ordering Update**: Calls `OrderingService.Configure()` with the new orderer endpoints
7. **Error Handling**: If any step fails, retries with exponential backoff
8. **Continue Monitoring**: After success or retry exhaustion, continues monitoring

## Error Handling

The service implements robust error handling:

- **Transient Errors**: Automatically retried with exponential backoff
- **Persistent Errors**: Logged and monitoring continues
- **Context Cancellation**: Gracefully stops on context cancellation
- **Retry Exhaustion**: After max retries, error is logged and monitoring continues

## Thread Safety

The service is thread-safe and can be safely used from multiple goroutines:

- Start/Stop operations are protected by mutex
- State changes are atomic
- Safe concurrent access to running status

## Testing

The package includes comprehensive test coverage:

```bash
# Run all tests
go test ./platform/fabricx/core/channelconfig/...

# Run with coverage
go test -cover ./platform/fabricx/core/channelconfig/...

# Run specific tests
go test -v -run TestNewConfig ./platform/fabricx/core/channelconfig/
```

### Mock Implementations

Mock implementations are generated using [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter):

```bash
# Generate mocks
go generate ./...
```

Use mocks in tests:

```go
import "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channelconfig/mock"

// Create mocks
queryService := &mock.QueryService{}
membershipService := &mock.MembershipService{}
orderingService := &mock.OrderingService{}
configService := &mock.ConfigService{}

// Configure mock behavior
queryService.GetConfigTransactionReturns(&channelconfig.ConfigTransactionInfo{
    Envelope: envelope,
    Version:  2,
}, nil)

// Use in tests
monitor, err := channelconfig.NewChannelConfigMonitor(
    config,
    queryService,
    membershipService,
    orderingService,
    configService,
    "testnet",
    "testchannel",
)
```

The following mocks are available:
- `mock.QueryService` - Mock for QueryService interface
- `mock.MembershipService` - Mock for MembershipService interface
- `mock.OrderingService` - Mock for OrderingService interface
- `mock.ConfigService` - Mock for driver.ConfigService interface

## Limitations

- **In-Memory State**: Configuration version tracking is in-memory only (resets on restart)
- **Single Channel**: One monitor instance per channel (multiple channels require multiple instances)
- **Polling-Based**: Uses polling instead of event-driven approach (may have slight delay)

## Future Enhancements

- Event-driven mode using block events
- Persistent state storage
- Metrics and monitoring
- Multi-channel support in single instance
- Adaptive polling intervals
- Configuration change notifications

## Dependencies

- Query Service: `platform/fabricx/core/vault/queryservice`
- Membership Service: `platform/fabricx/core/membership`
- Ordering Service: Fabric ordering service interface
- Config Service: Standard Fabric config service interface

## See Also

- [Design Document](./DESIGN.md) - Detailed design and architecture
- [Implementation Plan](./IMPLEMENTATION_PLAN.md) - Step-by-step implementation guide
- [Fabric-X Documentation](../../../docs/platform/fabric-x/readme.md)

## License

Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0