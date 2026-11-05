# Upgrading from v0.28 to v0.29

This guide takes an application built on FSC v0.28.0 to v0.29.0. Coming from an older release,
first follow [v0.27-to-v0.28.md](v0.27-to-v0.28.md).

Every organization upgrades in the same window (see [Storage](#storage)).

# For operators

## Storage

The vault stores metadata as protobuf.

# Checklist

Operators:

- Run `node migrate --contract` once no node runs v0.28; see [Storage](#storage) and
  [schema migrations](../platform/view/services/schema-migrations.md).

## Developers

- Nothing to change.
