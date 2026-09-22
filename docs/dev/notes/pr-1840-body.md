## Summary

`Committer.Commit` returning an error permanently stopped the channel's block
delivery. `Delivery` is single-use, nothing supervised it, and nothing reported the
stop — so one transient storage fault silently ended block ingestion for that
channel. Note the asymmetry: `runReceiver` already reconnects indefinitely on gRPC
failure, so stream faults self-healed while committer faults were fatal.

Classify the failure **in the committer** and act on the class:

- **retry** — non-deterministic and replay-safe (`DeadlockDetected`, `SqlBusy`, an
  expired context): commit the same block again, bounded, then escalate
- **degrade** — deterministic on an already-final block, such as a configuration
  this node cannot apply: return the error, stopping the channel, and count it
- **fatal** — an invariant is broken, so committing further blocks would build on
  state the node cannot vouch for

Classification lives in the committer because that is the component that can tell
the cases apart: it owns the vault, the membership service and the channel
configuration. `Commit` therefore returns only final errors, and `readBlocks` stops
on any error without inspecting one — which is what its own doc comment always
described.

Retrying needs no new idempotence work: `CommitEndorserTransaction` already skips
transactions marked valid or invalid, and `CommitConfig` skips a configuration
present in the vault.

A stopped channel emits no blocks and no further errors, so it was invisible in
every existing signal — `block_commit` simply stops receiving observations, which
looks identical to an idle channel. Two counters record it, labelled by `network`,
`channel` and failure `class`.

Also drops the comment at the callback site claiming a failed commit is retried,
and the same claim on `driver.BlockCallback`.

Fixes #1731
Fixes #1624

## Review feedback addressed

@mbrandenburger — the layering point was right; the rework is in this push.

- **classification belongs in the committer** — moved to `committer/failure.go`.
  `delivery` no longer imports `storage/driver` or `fabric/driver` at all. The clean
  confirmation: `runBlockScan` no longer needs `deliveryService.commitRetries = 0`,
  which existed only to undo the retry for application-supplied `Scan` callbacks
  where the idempotence guarantee does not hold. It deleted itself.
- **naming** — the field, config keys and counters are all committer-scoped now.
- **channel label** — added `channel` and `network`. The metrics provider is
  process-wide, so without them every channel incremented one series and an operator
  could see that *something* stopped but not what.
- **comment placement in `configuration.md`** — moved to the `committer:` block.

Net effect of the move: **−521 lines** against the previous iteration.

## Note for reviewers

#1731 states that `Commit` is not idempotent per block, which would make
replay-safety a prerequisite for any retry. It already holds: `CommitTX`'s
`"is already valid"` returns are unreachable assertion branches, because every
caller pre-checks vault status and `applyConfigCommit` forces `Busy` via
`Vault.NewRWSet` first. No idempotence work is needed, so none is here.

**One open decision.** #1624 suggests keeping non-config transactions flowing after
a config failure. This does the opposite: a configuration we failed to apply may
have rotated MSPs or changed endorsement policy, so committing later blocks would
validate against rules the network has already replaced. Stalling is the
conservative choice, and is only defensible because the failure is now counted.
Worth confirming before merge.

## Test plan

- [x] `go build ./...` (all four modules)
- [x] `go test -race -count=1 ./platform/fabric/core/generic/{committer,delivery,config,chaincode}/...`
      (committer 91.5%, delivery 94.3%)
- [x] `golangci-lint run ./platform/fabric/core/generic/{committer,delivery,config}/... ./platform/fabric/driver/...`
      (0 issues)
- [ ] No integration test: reproducing this needs Fabric binaries + Docker and a
      fault-injection seam into the vault. `integration/fabric/configupdate/`
      (#1695) is the natural home for the config path — follow-up.
