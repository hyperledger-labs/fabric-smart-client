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

## File Structure

```
platform/fabricx/core/channel/config/
├── DESIGN.md                    # This document
├── IMPLEMENTATION_PLAN.md       # Implementation plan
├── monitor.go                   # Main service implementation
├── monitor_test.go              # Unit tests
├── config.go                    # Configuration parsing
├── config_test.go               # Configuration tests
└── mock/                        # Mock implementations for testing
    ├── query_service.go
    ├── membership_service.go
    └── ordering_service.go
```

## Testing Strategy

### Unit Tests
1. Configuration parsing
2. Version comparison logic
3. Error handling and retry logic
4. Start/Stop lifecycle
5. Mock-based integration tests

### Integration Tests
1. End-to-end monitoring with real services
2. Configuration change detection
3. Service update verification
4. Error recovery scenarios

## Security Considerations

1. **Access Control**: Service uses existing query service authentication
2. **Configuration Validation**: Validate config transactions before applying
3. **Logging**: Avoid logging sensitive configuration data
4. **Error Messages**: Don't expose internal system details in errors

## Performance Considerations

1. **Polling Overhead**: Configurable interval allows tuning
2. **Memory Usage**: Minimal - only tracks last config version
3. **Network Traffic**: One query per poll interval per channel
4. **CPU Usage**: Negligible - mostly idle waiting

## Future Enhancements

1. **Event-Driven Mode**: Listen for block events instead of polling
2. **Persistent State**: Store last processed version in database
3. **Metrics**: Expose monitoring metrics (updates, errors, latency)
4. **Multi-Channel Support**: Single service monitoring multiple channels
5. **Smart Polling**: Adaptive interval based on network activity
6. **Notification System**: Emit events when configuration changes

## Migration Path

1. Service is opt-in - existing systems continue working
2. Enable per-channel via configuration
3. No database migrations required
4. Backward compatible with existing services

## Acceptance Criteria

- [ ] Service can be started and stopped via API
- [ ] Service polls at configured interval
- [ ] Service detects new configuration versions
- [ ] Service updates membership service on config change
- [ ] Service updates ordering service on config change
- [ ] Service handles errors with exponential backoff
- [ ] Service logs all operations appropriately
- [ ] Service passes all unit tests
- [ ] Service passes integration tests
- [ ] Configuration is properly documented

## References

- Hyperledger Fabric Configuration Transaction Format
- Fabric Smart Client Architecture
- Existing Membership Service Implementation (`platform/fabricx/core/membership`)
- Existing Query Service Implementation (`platform/fabricx/core/vault/queryservice`)