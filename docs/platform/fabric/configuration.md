# Fabric Platform Configuration

The Fabric platform configuration lives inside the shared FSC `core.yaml` described in the [shared node configuration](../../configuration.md) guide.

## Start with the Shared Configuration

Before adding Fabric-specific sections, configure the common FSC node settings:

- `logging`
- `fsc`
- `grpc`
- `p2p`
- `persistences`
- `web`
- `tracing`
- `metrics`

## Fabric-Specific Settings

When a node uses the Fabric platform, the `core.yaml` also includes `fabric` network definitions and related services such as:

- network connection settings
- identities and MSP material
- finality handling
- vault and persistence wiring
- channel and chaincode access configuration

Use the full [example configuration](../../configuration.md) as the starting point for a Fabric-enabled FSC node.

## Related Documentation

- [Fabric SDK architecture](fabric-sdk.md)
- [Fabric platform driver architecture](drivers.md)
