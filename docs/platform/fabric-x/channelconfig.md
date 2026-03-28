# Channel Configuration Monitor Service

## Overview

The Channel Configuration Monitor Service is a service under `platform/fabricx/core/channel/config` that periodically monitors for new channel configuration transactions and automatically updates the membership and ordering services when configuration changes are detected.

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

## Configuration

### Configuration Structure

The Channel Configuration Monitor is configured per channel within the FabricX network configuration. All configuration parameters are optional and have sensible defaults.

**Note:** This configuration applies **only to FabricX** networks, not standard Fabric networks.

```yaml
fabric:
  <network>:
    # Channel Configuration Monitor settings (applies to all channels in this network)
    configMonitor:
      # How often to check for configuration updates
      # Default: 1m (1 minute)
      # Format: Go duration string (e.g., "30s", "2m", "1h")
      pollInterval: 60s
      
      # Maximum number of retry attempts for failed operations
      # Default: 5
      # Set to 0 to disable retries
      maxRetries: 5
      
      # Initial delay before the first retry attempt
      # Default: 1s
      # Format: Go duration string
      initialRetryDelay: 1s
      
      # Maximum delay between retry attempts (exponential backoff cap)
      # Default: 5m
      # Format: Go duration string
      maxRetryDelay: 5m
    
    channels:
      - name: <channel>
        # ... other channel configuration
```

### Configuration Examples

#### Minimal Configuration (Using Defaults)

If you don't specify the `configMonitor` section, the service will use default values:

```yaml
fabric:
  mynetwork:
    # configMonitor section omitted - uses defaults
    channels:
      - name: mychannel
```

#### Custom Configuration for High-Frequency Monitoring

For environments where configuration changes need to be detected quickly:

```yaml
fabric:
  mynetwork:
    configMonitor:
      pollInterval: 10s        # Check every 10 seconds
      maxRetries: 10           # More retry attempts
      initialRetryDelay: 500ms # Faster initial retry
      maxRetryDelay: 1m        # Lower max delay
    channels:
      - name: mychannel
```

#### Conservative Configuration for Stable Networks

For production environments with infrequent configuration changes:

```yaml
fabric:
  mynetwork:
    configMonitor:
      pollInterval: 5m         # Check every 5 minutes
      maxRetries: 3            # Fewer retries
      initialRetryDelay: 2s    # Longer initial delay
      maxRetryDelay: 10m       # Higher max delay
    channels:
      - name: mychannel
```

### Environment Variable Overrides

Configuration values can be overridden using environment variables. The pattern follows:

```
CORE_FABRIC_<NETWORK>_CONFIGMONITOR_<PARAMETER>
```

**Examples:**

```bash
# Override poll interval for 'mynetwork' network
export CORE_FABRIC_MYNETWORK_CONFIGMONITOR_POLLINTERVAL=30s

# Override max retries
export CORE_FABRIC_MYNETWORK_CONFIGMONITOR_MAXRETRIES=10

# Override initial retry delay
export CORE_FABRIC_MYNETWORK_CONFIGMONITOR_INITIALRETRYDELAY=2s

# Override max retry delay
export CORE_FABRIC_MYNETWORK_CONFIGMONITOR_MAXRETRYDELAY=10m
```

**Note:** Network names in environment variables should be uppercase.

### Configuration Validation

The service validates configuration parameters on startup:

| Parameter | Validation Rule | Error if Invalid |
|-----------|----------------|------------------|
| `pollInterval` | Must be > 0 | "pollInterval must be positive" |
| `maxRetries` | Must be >= 0 | "maxRetries must be non-negative" |
| `initialRetryDelay` | Must be > 0 | "initialRetryDelay must be positive" |
| `maxRetryDelay` | Must be > 0 | "maxRetryDelay must be positive" |
| `initialRetryDelay` vs `maxRetryDelay` | initialRetryDelay <= maxRetryDelay | "initialRetryDelay must not exceed maxRetryDelay" |

If validation fails, the node will fail to start with a descriptive error message.

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
