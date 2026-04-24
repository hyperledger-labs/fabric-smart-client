# Fabric-x Platform Configuration

The Fabric-x platform builds on the shared FSC configuration plus additional `fabric.<network>` service settings for Fabric-x-specific integrations.

## Shared Node Configuration

Start with the common FSC node settings documented in the [shared node configuration](../../configuration.md) guide.

## Fabric-x Services

The Fabric-x platform currently documents configuration for these services:

- `fabric.<network>.notificationService`
- `fabric.<network>.queryService`

Both services support endpoint-based gRPC connectivity and optional TLS or mutual TLS settings.

## Related Documentation

- [Fabric-x platform overview](README.md)
- [Fabric platform configuration](../fabric/configuration.md)
