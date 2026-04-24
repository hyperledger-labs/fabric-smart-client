# View Platform Configuration

The View platform uses the shared FSC node configuration described in [shared node configuration](../../configuration.md).

## Core Sections

For a node using only the View platform, the most relevant `core.yaml` sections are:

- `logging`
- `fsc`
- `grpc`
- `p2p`
- `kvs`
- `persistences`
- `web`
- `tracing`
- `metrics`

These sections configure node identity, transport, persistence, observability, and runtime services used by the View SDK.

## Related Documentation

- [View SDK architecture](view-sdk.md)
- [View runtime security model](security-model.md)
