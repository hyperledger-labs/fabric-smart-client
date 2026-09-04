# FabricX ATSA (idemix) integration test

This suite runs the **A**sset **T**ransfer **S**ecured **A**greement flow
(`Issue → AgreeToSell → AgreeToBuy → Transfer`) on the **FabricX** platform, reusing the
`integration/fabric/atsa` views **unmodified** and exercising **idemix** (`alice`/`bob` run with
`fabric.WithAnonymousIdentity()`, transactions built via `state.NewAnonymousTransaction`).

## What it validates

The Transfer step depends on two state-service features that Fabric provides but FabricX
historically did not. Both are now supported below the view layer:

1. **Hash-hidden state metadata.** `AgreeToSell`/`AgreeToBuy` write outputs with
   `state.WithHashHiding()` (whole-state: on-ledger value = `sha256(json)`, preimage kept as
   off-ledger field-mapping); the `Asset` hides only its `PrivateProperties` field
   (`state:"hash"`, field-level). FabricX FSC nodes are stateless (reads go to the committer
   QueryService, which returns only `{Raw, Version}`), so the preimage is persisted **synchronously
   on the endorsement path** (`Transaction.StoreTransient`) into a new
   `(network, channel, ns, key, sha256(committed value))`-indexed store and served back on a vault
   metadata miss.

2. **State certification.** `WithCertification()` / `VerifyCertification()` are backed by a
   `TrustedReadCertifier` (resolved per network via the `state.Certifier` seam, registered by the
   fabricx SDK) that trusts the vault's committed read — served by the committer QueryService —
   instead of chaincode endorsement.

See `docs/superpowers/specs/2026-07-22-fabricx-hashhiding-certification-design.md` for the full
design.

## Running

Fabric-x suites shell out to `configtxgen`/`fxconfig` from `$FAB_BINS/fabric-x`, where
`make install-fabricx-tools` installs them. The Makefile default for `FAB_BINS` is `PWD`-relative
and resolves incorrectly from a linked worktree, so pass an absolute path:

```bash
make integration-tests-fabricx-atsa \
  FAB_BINS=/absolute/path/to/hyperledger-labs/fabric/bin
```
