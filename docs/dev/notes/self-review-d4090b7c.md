# Self-review — `d4090b7c` (pushed)

Reviewing my own implementation after the move into the committer. Two real
defects, three things worth improving, and a few notes. No changes made.

Commit: `fix(fabric/committer): retry transient commit failures instead of stopping the channel`
Branch: `fix/1731-commit-failure-taxonomy` @ `d4090b7c`, matches `origin`.

| # | Finding | Severity |
| --- | --- | --- |
| **B1** | A retry re-publishes finality events for transactions that already committed | **bug** |
| **B2** | The cancellation path reports `class="retry"`, which the docs say never happens | **bug** |
| I1 | `commitBlock` test seam is production state | improve |
| I2 | Retry sleep is not interruptible by `Stop`, only by context | improve |
| I3 | `fabricx` silently inherits the retry | verify |
| N1..N4 | Notes, no action needed | — |

---

## B1 — a retry re-publishes finality events (bug)

**The defect.** `commitTxs` runs transaction groups in parallel through an
`errgroup`, and each successful transaction pushes to `c.events`:

```go
eg.SetLimit(c.ChannelConfig.CommitParallelism())
for _, txGroup := range parallelizableTxGroups {
    eg.Go(func() error {
        for _, tx := range txs {
            ...
            c.events <- *event          // published per successful tx
        }
    })
}
err := eg.Wait()                        // one group failing fails the block
```

`eg.Wait()` returns the first error, but the groups that *succeeded* have already
published their events. When `retryBlock` commits the block again, those
transactions run a second time and publish a **second** event each.

**Why the idempotence argument does not cover this.** The argument I relied on —
and stated in the commit message — is about the *vault*:
`CommitEndorserTransaction` checks status and returns `(true, nil)` for
`Valid`/`Invalid`, so no double-write happens. That part holds. But it returns
`processed = true` with a non-nil `event`, and `commitTxs` publishes any non-nil
event regardless. So the vault is idempotent and the **event stream is not**.
Replay-safety at the storage layer is not replay-safety at the notification
layer, and I conflated them.

**What a consumer sees.** Tracing the three consumers in `runEventNotifiers`:

| Consumer | Duplicate-safe? | Why |
| --- | --- | --- |
| `FinalityManager.Post` | **yes** | `cloneListeners` does `delete(c.txIDListeners, txID)`, so listeners fire once |
| `notifyFinality` | tolerable | channels are buffered at 100 and deleted on return from `listenTo`, so a duplicate is absorbed, not deadlocked |
| `notifyTxStatus` | **no** | publishes `TransactionStatusChanged` to the event bus unconditionally, twice |

So the blast radius is application code subscribed to transaction-status events:
it can observe the same transaction reaching `Valid` twice. Not corruption, and
no deadlock, but a contract an application may reasonably not expect — and
nothing documents that it can happen.

**Worth noting this is not new to my change**: the delivery-layer version had the
same property, and so does the pre-existing stream-reconnect path, where
`GetStartPosition` replays the last block. What my change does is make it *much*
more frequent — every transient storage fault now replays a block, where before
only a reconnect did.

**Options.** Not obvious which is right, which is why this is a report and not a
patch:

1. **Deduplicate at the publish site.** Skip publishing when the handler reports
   the transaction was already processed. `CommitEndorserTransaction` already
   returns that flag; it is discarded in the `HandleEndorserTransaction` path.
   Smallest change, but needs care that a genuinely-first commit still publishes.
2. **Retry at transaction granularity** rather than block granularity, so only
   the failed group re-runs. Truer to the intent, considerably more work, and it
   interacts with the dependency resolver's grouping.
3. **Document it as acceptable** and state in `FinalityListener`/the status-event
   contract that a status may be delivered more than once. Cheapest, and
   arguably honest since the reconnect path already permits it — but it is a
   contract change that should be a maintainer's call, not mine.

My inclination is (1), with a test that pins single-publish across a retry. But
this needs Marcus's view, because (3) may be the project's existing position.

---

## B2 — the cancellation path reports `class="retry"` (bug)

`retryBlock`'s pre-attempt guard returns `ctx.Err()` bare:

```go
if err := ctx.Err(); err != nil {
    return err                     // plain context.Canceled
}
```

`classify` maps `context.Canceled` to `classRetry`. So `Commit` logs and
increments `commit_failures{class="retry"}` — the one label
`monitoring_metrics.md` states never appears there:

> `retry` never appears here: a retryable failure that exhausts its budget is
> wrapped in `ErrRetriesExhausted`, which classifies as `degrade`.

Verified by running the existing test:

```
ERRO … Commit -> commit failed for block [13] with class [retry], not retrying further: [context canceled]
--- PASS: TestRetryBlockTransient/abandons_retries_when_the_caller's_context_is_cancelled
```

**The test passes because it never asserts the class on that path** — it checks
only `require.Error` and the call count. That is precisely the gap that let the
same class of defect through in the previous iteration, and I did not close it
here.

Note the *other* cancellation exit, inside the sleep `select`, does wrap
correctly in `ErrRetriesExhausted`, so the two exits from the same loop disagree
with each other. That inconsistency is the smell.

**Fix direction.** Either wrap this exit like the other one, or — better — do not
count a cancellation as a commit failure at all. A caller cancelling is not the
channel failing, and inflating `commit_failures` with shutdowns makes the alert
noisy at exactly the wrong moment. That argues for returning early in `Commit`
before the counter, when `errors.Is(err, context.Canceled)` and the caller's
context is done.

---

## I1 — the `commitBlock` test seam is production state

```go
// commitBlock substitutes for one attempt at committing a block. It is nil in
// production, where commitOnce runs the real path; a test sets it to drive
// retryBlock's classification and budget without standing up a vault.
commitBlock func(context.Context, *common.Block) error
```

A nil-checked function field on the production struct, existing only for tests,
plus a `commitOnce` indirection that exists only to check it. I added it because
driving `retryBlock` through the real path needs a vault, an envelope service, a
processor manager and a dependency resolver.

It works and it is documented, but it is the same category of thing the previous
review flagged as nits (`if d.metrics != nil`, the nil-`channelConfig` branch) —
test-shaped structure in production code. Cleaner alternatives:

- Extract `retryBlock` to take a `func(context.Context, *common.Block) error`
  parameter, so the test passes its stub as an argument and the struct field
  disappears.
- Or build a real `Committer` with the existing `fake` package, if the fakes
  cover enough of the commit path.

The first is a small change and removes both the field and `commitOnce`.

---

## I2 — the retry sleep is not interruptible by `Stop`

Between attempts, `retryBlock` waits on `ctx.Done()` or the sleep timer. That is
correct as written — but the *delivery* service's shutdown signal is `d.stop`,
not a context, and the committer has no access to it. Sequence:

1. `Delivery.Stop(nil)` is called during node shutdown.
2. `readBlocks` is blocked inside `d.callback(...)` → `Commit` → sleeping.
3. The sleep runs its full `CommitRetrySleep` (10s default) before noticing
   nothing.

With the defaults a shutdown can be delayed by up to ~10s per in-flight block,
and up to ~50s if the budget keeps failing. The context *usually* covers this,
because `Run`'s context is typically cancelled at shutdown too — but `Stop(nil)`
alone does not cancel it, and `Delivery.Stop` is documented as callable
independently.

Not a correctness bug; a shutdown-latency one. Worth measuring before changing
anything, and worth a sentence in `Commit`'s Godoc noting the caller should
cancel its context, not only stop its stream.

---

## I3 — `fabricx` inherits the retry silently

`platform/fabricx/core/committer/txhandler.go` calls
`RegisterTransactionHandler(com *committer.Committer)`, registering its handler
on the **same** `Committer` type. So fabricx now retries transient commit
failures too, with no fabricx-side change and no mention in the commit message.

Probably desirable — it is the same defect class. But it is untested on that
path, and `HandleFabricxTransaction` has its own error shapes
(`committerpb.Status`) that `classify` has never been checked against. Should be
verified deliberately rather than inherited by accident.

---

## Notes, no action

**N1 — the workaround really did delete itself.** `runBlockScan` no longer needs
`commitRetries = 0`, and `TestScanDoesNotRetry` was removed because scans cannot
retry when nothing in delivery retries. That is the clean confirmation the move
was correct, and it is the strongest evidence for Marcus's original point.

**N2 — the coupling is verifiably gone.** No reference to `storage/driver`,
`ErrConfigRejected`, `DeadlockDetected`, `SqlBusy` or `classify` remains anywhere
in `platform/fabric/core/generic/delivery/`. Checked by grep, not by assumption.

**N3 — not a breaking change, and the message no longer claims it is.** All five
symbols I had listed as breaking (`DeliveryCommitRetries`, `delivery.Metrics`,
`NewMetrics`, `FatalHandler`, `ErrFatalCommit`) are absent from `main`; they only
ever existed on this branch. Against `main` the `ChannelConfig` change is purely
additive.

**N4 — the `FatalHandler` hook is gone, not moved.** Nothing installed one, so I
dropped it rather than re-plumb an unused hook through a new layer. The `fatal`
class and `ErrFatalCommit` still exist on the committer, so re-adding is easy —
but #1624 explicitly asked for a "deliberate, documented process exit" as one
option, so this should be raised with Marcus rather than left silent.

---

## Verification state

- `go build ./...` clean across all four modules.
- `-race -count=1` green on `committer`, `delivery`, `config`, `chaincode`.
- `golangci-lint` 0 issues on the touched packages.
- Coverage: committer 91.5%, delivery 94.3%.
- Net −521 lines against the previous iteration; +820/−17 against `main`.

**Not verified:** no integration test. Both B1 and B2 are the kind of thing an
integration test with real fault injection would surface and unit tests did not.

## What I would do next, in order

1. **B2** — smallest, clearly wrong, one-line-ish fix plus the class assertion
   the test is missing.
2. **B1** — needs a decision on which of the three options, so it needs Marcus.
3. **I1** — small cleanup, removes test-shaped code from the production struct.
4. **I3** — verify rather than assume fabricx behaves under the new retry.
5. Rebase onto `main`, which is 6 commits ahead and includes two
   `refactor(utils)!` changes.
