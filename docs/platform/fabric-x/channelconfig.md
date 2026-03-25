# Channel Configuration Monitor Service

## Overview

The Channel Configuration Monitor Service is a new service under `platform/fabricx/core/channel/config` that periodically monitors for new channel configuration transactions and automatically updates the membership and ordering services when configuration changes are detected.

## Problem Statement

In Hyperledger Fabric networks, channel configuration can change over time (e.g., new organizations joining, orderer endpoints being updated). Currently, there is no automated mechanism to detect and apply these configuration changes. This service addresses this gap by:

1. Periodically checking for new configuration transactions using the query service
2. Detecting configuration version changes
3. Updating the membership service with new MSP configurations
4. Updating the ordering service with new orderer endpoints

## Architecture

### Components

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

### Service Interface

```go
type ChannelConfigMonitor interface {
    // Start begins monitoring for configuration changes
    Start(ctx context.Context) error
    
    // Stop halts the monitoring process
    Stop() error
    
    // IsRunning returns the current state of the monitor
    IsRunning() bool
}
```

## Design Decisions

### 1. Polling Interval
- **Decision**: Configurable interval with 1-minute default
- **Rationale**: Balances responsiveness with network overhead
- **Configuration**: `fabric.<network>.channels.<channel>.configmonitor.pollInterval`

### 2. State Persistence
- **Decision**: In-memory only (resets on restart)
- **Rationale**: Simplifies implementation; on restart, the service will check current config and update if needed
- **Implementation**: Track last processed config version in memory

### 3. Error Handling
- **Decision**: Retry with exponential backoff, log errors, continue monitoring
- **Rationale**: Transient network issues shouldn't stop monitoring; persistent errors are logged for investigation
- **Parameters**:
  - Initial retry delay: 1 second
  - Max retry delay: 5 minutes
  - Max retries per cycle: 5

### 4. Lifecycle Management
- **Decision**: Explicit Start/Stop API calls
- **Rationale**: Provides fine-grained control over when monitoring occurs
- **Usage**: Caller controls when to start/stop monitoring

### 5. Channel Scope
- **Decision**: One service instance per channel
- **Rationale**: Simplifies implementation and allows per-channel configuration
- **Implication**: Multiple channels require multiple monitor instances

## Implementation Details

### Configuration Structure

```yaml
fabric:
  <network>:
    channels:
      <channel>:
        configmonitor:
          pollInterval: 60s  # Default: 1 minute
          maxRetries: 5      # Default: 5
          initialRetryDelay: 1s
          maxRetryDelay: 5m
```

### Key Algorithms

#### 1. Monitoring Loop
```
1. Wait for poll interval
2. Query for latest config transaction (GetConfigTransaction)
3. Compare version with last processed version
4. If new version detected:
   a. Update membership service with envelope
   b. Extract orderer config from membership service
   c. Update ordering service with orderer config
   d. Update last processed version
5. On error:
   a. Log error
   b. Apply exponential backoff
   c. Retry up to max retries
   d. Continue monitoring after retry exhaustion
6. Repeat from step 1
```

#### 2. Configuration Update Process
```
1. Call QueryService.GetConfigTransaction()
2. Extract version from ConfigTransactionInfo
3. If version > lastVersion:
   a. Call MembershipService.Update(envelope)
   b. Call MembershipService.OrdererConfig(configService)
   c. Call OrderingService.Configure(consensusType, orderers)
   d. Update lastVersion
```

### Error Scenarios

| Scenario | Handling |
|----------|----------|
| Query service unavailable | Retry with backoff, log error, continue |
| Invalid config transaction | Log error, skip update, continue monitoring |
| Membership update fails | Retry with backoff, log error, continue |
| Ordering service update fails | Retry with backoff, log error, continue |
| Context cancelled | Clean shutdown, stop monitoring |

### Dependencies

- **Query Service** (`platform/fabricx/core/vault/queryservice`): For retrieving configuration transactions
- **Membership Service** (`platform/fabricx/core/membership`): For updating MSP configurations and extracting orderer config
- **Ordering Service**: For updating orderer endpoints
- **Config Service**: For reading configuration parameters
- **Logger**: For logging operations and errors
