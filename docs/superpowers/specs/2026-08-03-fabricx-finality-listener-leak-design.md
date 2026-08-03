# Fabric-x finality listener leak — design

**Issue:** [#1626](https://github.com/hyperledger-labs/fabric-smart-client/issues/1626) (`bug`, `Fabric-x`, opened 2026-07-30 by mbrandenburger)
**Status:** Design complete, awaiting review. Not implemented.
**Target package:** `platform/fabricx/core/finality`

## Problem

`notificationListenerManager.handlers` (`nlm.go:41`) maps txID → listeners. `AddFinalityListener`
writes an entry (`nlm.go:225`) then subscribes to the remote committer for that transaction's
status (`nlm.go:256`).

In steady state the **only** path that deletes an entry is a notification arriving back from the
committer (`nlm.go:134`). The other three removal paths are edge cases:

| Line | Path | Trigger |
|------|------|---------|
| `nlm.go:134` | dispatcher | notification arrived ← **only steady-state path** |
| `nlm.go:259` | `AddFinalityListener` | rollback when the *send itself* failed |
| `nlm.go:296` | `RemoveFinalityListener` | explicit caller removal |
| `nlm.go:171` | `listen()` teardown | `clear(n.handlers)` when the stream dies |

The public contract (`platform/fabric/committer.go:42-45`) states:

> *"When the listener is invoked, then it is also removed. The transaction id must not be empty."*

So cleanup is the framework's job and callers correctly do not unregister — e.g.
`integration/fabricx/simple/views/create.go:94` registers and never removes. When no notification
arrives, **neither side cleans up**.

### The 10s timeout is not a local timeout

`nlm.go:234-240` looks like a safety net but is a field *inside the outbound request*, asking the
committer to give up and reply with `TimeoutTxIds`. It is delivered over the same stream whose
silence is the problem. There is no `time.Timer`/`time.Ticker` anywhere in the file.

### What actually leaks

The map entry is ~150 bytes (64-char txID key + slice header). The real cost is the **retained
listener closure**. The in-tree `IsFinal` closure captures a buffered channel
(`core/channel/wrappers.go:68-71`); application listeners can capture arbitrarily more. `len(handlers)`
therefore understates retained memory.

**Counterintuitive mitigation:** `clear(n.handlers)` on stream teardown (`nlm.go:170-172`) means a
node whose stream *flaps* self-heals. A node with a **stable, healthy stream** is the one that
accumulates.

## Findings beyond the issue

Established by reading `notify.proto` (`fabric-x-common@v0.2.8`) and the query service. These change
the fix shape and are the reason we are not implementing the issue's proposal verbatim.

### 1. `RejectedTxIds` is never read — a fourth leak path

`NotificationResponse` has three fields (`notify.proto:42-46`): `tx_status_events`, `timeout_tx_ids`,
and `rejected_tx_ids` (with a `reason` string, `notify.proto:49-52`).

`parseResponse` (`nlm.go:178-208`) reads only the first two. `grep` for
`RejectedTxIds|GetRejectedTxIds` across `platform/fabricx/` returns **zero non-test hits**.

So when the committer explicitly tells us a subscription was rejected, we discard the message and
leak the entry permanently. **No server fault is required** — rejection is correct server behaviour.
The discarded `reason` is also exactly the `statusMessage` that `OnStatus` accepts and `IsFinal`
would surface (`wrappers.go:83`), currently always `""` (`nlm.go:151`).

### 2. The remote timeout is explicitly non-strict

`notify.proto:32-34`:

> *"The timeout duration that applies to ALL the subscriptions in this request. It is **non-strict**,
> i.e., it is possible to receive notifications for this request **after the timeout has passed**."*

Cuts both ways, and both matter:

- **Strengthens** the case for a local TTL — the proto itself disclaims bounded reply time.
- **But** a naive TTL races the remote. Expire locally, committer legitimately notifies later → we
  already told the caller `Unknown` for a transaction that **committed**. The proto sanctions
  exactly this late delivery.

This finding is what drives the sweeper design below: we **query at expiry instead of guessing**, so
the race cannot produce a wrong answer.

### 3. Hardcoded timeout overrides a deployment-tuned default

`notify.proto:35-36`: if `timeout` is unset/zero, the committer applies its own configured default.
FSC hardcodes 10s with a `// TODO: set a proper timeout` (`nlm.go:238-239`), overriding a
server-side default with a guess. Out of scope here; see [Out of scope](#out-of-scope).

### 4. `GetTransactionStatuses` is purpose-built for this

`core/committer/queryservice/query.go:95-99`:

> *"Unlike `GetTransactionStatus`, which returns an error for a transaction unknown to the committer,
> unknown transactions are **omitted** from the result so callers can treat a missing entry as
> **'not final yet'**."*

Batch-capable (one gRPC call for N txIDs, `query.go:100-132`), and "absent = not yet final" is
precisely the semantics expiry needs — no error-string parsing, no distinguishing "unknown tx" from
"query failed".

**Wiring is clean:** `queryservice.Provider` is already registered in the same `dig` container as
`finality.NewListenerManagerProvider` (`sdk/dig/sdk.go:62,64`), both are per-network, and
`queryservice.QueryService` is already an interface (`queryservice/provider.go:37`).

### 5. `RemoteQueryService` ignores the caller's context

`query.go:78,115,136` all use `context.Background()` with `config.RequestTimeout`
(`DefaultRequestTimeout = 30s`, `config.go:16`). Queries are therefore **uncancellable** and can
stall up to 30s.

This is what rules out a synchronous query on the registration hot path — see
[Rejected: query at registration](#rejected-query-at-registration).

## Approach: exact fixes first, query-backed expiry as backstop

The issue bundles every leak under "add a TTL". We are not doing that, because the leak paths fall
into two classes that want different treatment:

**Class A — deterministic, no server fault needed.** Empty txID; rejected subscriptions. These have
*exact* causes and *exact* fixes. A TTL "fixes" them only by waiting and then reporting `Unknown` —
wrong answer, delivered slowly, for cases we can answer correctly and immediately.

**Class B — non-deterministic.** Committer genuinely never emits (overload, bug, dropped
subscription), or the transaction was already final before registration. Only a local timer can
bound this.

So: fix Class A precisely; use the timer **only** for Class B — and have it *query* rather than
guess, which neutralises finding #2.

### Layer 1 — Reject empty txID

`AddFinalityListener` guards only `listener == nil` (`nlm.go:212-214`). An empty txID creates
`handlers[""]` and subscribes for `TxIds: [""]`; no committer will ever return a status for `""`, so
the entry is immortal by construction.

The generic driver **does** guard this (`platform/common/core/generic/committer/listenermgr.go:55-57`)
and `integration/fabric/iou/views/approver.go:95` asserts on that error. Fabric-x accepting it
silently is a cross-driver behavioural inconsistency on the same public API.

```go
if len(txID) == 0 {
    return errors.New("tx id must be not empty")
}
```

Returns before any state is touched: no map entry, no request sent. Wording matches the generic
driver.

**Caveat on cross-driver parity.** The error reaches callers on the direct path
(`finalityServiceAdapter.IsFinal` → `manager.AddFinalityListener`, `wrappers.go:104`), but
`committerService.AddFinalityListener` (`wrappers.go:237-244`) returns `nil` when its type assertion
on `finalityService` fails, so through *that* wrapper the error is swallowed regardless of this fix.
Not a blocker and not introduced here, but it means parity with the generic driver holds for the
manager, not unconditionally for every Fabric-x entry point.

### Layer 2 — Handle `RejectedTxIds`

**Rejected → `fdriver.Invalid`, not `Unknown`.** A rejection is definitive: the committer is saying
this transaction will never be processed. `Unknown` means "not yet known" and would leave callers
thinking a retry might resolve it.

**Signature change.** `parseResponse` returns `map[string]int` today — status only, nowhere to put
the reason. It becomes:

```go
type txOutcome struct {
    status  int
    message string
}

func parseResponse(resp *committerpb.NotificationResponse) map[string]txOutcome
```

Both are package-private with a single caller (`nlm.go:121`), so nothing outside `nlm.go` changes.
This is what lets the server's reason reach `OnStatus` instead of the current hardcoded `""`
(`nlm.go:151`); `IsFinal` already formats `statusMessage` into its error (`wrappers.go:83`), so the
reason surfaces to callers with no change there.

`RejectedTxIds` carries `tx_ids []string` plus **one shared** `reason` (`notify.proto:49-52`), so
every txID in the batch gets the same message. `resp.GetRejectedTxIds().GetReason()` is nil-safe in
generated Go — no explicit nil check needed.

**Explicit precedence.** Today `parseResponse` handles timeouts then status events and lets the later
write clobber the earlier (`nlm.go:181-205`) — so "status beats timeout" holds, but only as a side
effect of statement order. A third field makes that fragile. State it outright, weakest to strongest:

1. **Timeout** → `Unknown`
2. **Rejection** → `Invalid` + reason
3. **Status event** → mapped status

A definitive commit status always wins; a rejection beats a timeout. Worth a comment, since the next
person to add a response field needs to know.

### Layer 3 — Query-backed expiry (sweeper)

Per-entry deadline. On expiry, **ask the committer for the real status** rather than reporting a
blind `Unknown`. This merges what the issue treated as two separate items (local TTL, and the
"already final" check) into one mechanism.

**Placement: inside the existing dispatcher goroutine** (`nlm.go:106-164`), as a third `select` case:

```go
ticker := time.NewTicker(n.sweepInterval)
defer ticker.Stop()

for {
    select {
    case <-gCtx.Done():
        return gCtx.Err()
    case resp = <-n.responseQueue:
        // ... existing dispatch path
    case <-ticker.C:
        n.sweepExpired(gCtx)
    }
}
```

The dispatcher is the only writer that deletes on the notification path (`nlm.go:134`), so a sweep
cannot interleave with a dispatch. That eliminates "notification and expiry race for the same entry"
**by construction** rather than by locking discipline.

**`sweepExpired` — delete before query.** Follows the collect-under-lock-then-act-outside pattern the
dispatcher already uses (`nlm.go:126-139`):

1. **Under lock:** scan for `expiresAt` in the past, collect them, `delete` from the map. Unlock.
2. **Outside lock:** one `GetTransactionStatuses(expiredTxIDs)` call.
3. **Outside lock:** invoke listeners — present in result → true status; absent → `Unknown`.

Deletion happens in step 1, *before* the query, deliberately:

- **Cleanup becomes unconditional.** The bug being fixed is that removal depends on a remote party
  cooperating. Deleting after a successful query would re-create that structure — and worse, the
  query service and notification stream both talk to the same committer, so the failure that caused
  the missed notification is **correlated** with the query failing. The scenario where expiry is
  most needed is the one where the query is least likely to succeed.
- **It separates two concerns with different criticality.** Removing the entry bounds memory and must
  always happen; learning the true status is quality-of-answer and may fail harmlessly.
- **No revalidation window.** Query-before-delete would have to release the lock for I/O then
  re-acquire, during which a real notification may have removed the entry or a new listener may have
  joined that txID — requiring a re-check. One lock acquisition avoids that entirely.
- **Bounds the hang case.** Per finding #5 a query can stall 30s uncancellably; delete-first has
  already freed the entry and closure.

Accepted cost: once deleted the entry cannot be retried, so a failed query means `Unknown` with no
second chance. Acceptable because `Unknown` is a documented outcome the remote's own `TimeoutTxIds`
path already produces (`nlm.go:182-184`) and `IsFinal` already handles (`wrappers.go:84-86`) — and a
caller who receives it is *unblocked*, which is what matters. "Guaranteed unblocked, possibly vague"
beats "possibly never unblocked, precise".

**Degradation:** query error → log at warn, `Unknown` for the whole batch. Nil query service → skip
the query, `Unknown`. Both reduce to the issue's original proposal rather than failing.

**Handler invocation** reuses the existing goroutine-per-handler wrapper with its `handlerTimeout`
guard (`nlm.go:144-162`), so a hostile listener cannot stall the sweeper any more than it can stall
the dispatcher today.

**Granularity:** worst-case lifetime is `listenerTTL + sweepInterval`. The TTL is not a precise
deadline and tests must not assert as if it were.

### Layer 4 — State and zero-value contract

```go
type handlerEntry struct {
    listeners []fabric.FinalityListener
    expiresAt time.Time
}

type notificationListenerManager struct {
    // ... existing fields
    handlers      map[driver.TxID]*handlerEntry  // was map[TxID][]FinalityListener
    queryService  TxStatusQuerier                // nil ⇒ no query on expiry
    listenerTTL   time.Duration                  // 0 ⇒ expiry disabled
    sweepInterval time.Duration
}
```

**Zero value means disabled, not "expire immediately".** This is load-bearing:

- `setupTest` (`nlm_test.go:108-113`) builds the struct as a literal and sets no durations, so all
  existing tests get `listenerTTL == 0` and are **behaviourally** unaffected — no sweeper
  interference. (They still need mechanical edits for the map type change; see below.)
- A missing wire-up in `provider.go` degrades to today's behaviour rather than expiring every
  listener instantly — failing in the safe direction.
- TTL tests opt in explicitly, which documents intent at the call site.

**Map type change has real test churn.** Switching `map[TxID][]FinalityListener` →
`map[TxID]*handlerEntry` breaks every test that touches the map directly — roughly 18 sites:

- seeding: `nlm_test.go:200,248,526,573,648,712,788,789`
- asserting slice length / element: `nlm_test.go:339,380,534,582`
- plus `nlm_deadlock_poc_test.go:84`

All mechanical, but it is not zero work and must be budgeted in the implementation plan. Consider a
small test helper (`seedHandlers(nlm, txID, listeners...)`) to localise the change and keep future
shape changes cheap.

Same for `queryService == nil`: skip and fall back to `Unknown`, so the sweeper is testable without a
query mock and the two features are independently deployable.

Defaults, alongside `DefaultHandlerTimeout` (`nlm.go:33`):

```go
const DefaultListenerTTL   = 2 * time.Minute
const DefaultSweepInterval = 30 * time.Second
```

2 minutes is deliberately far larger than the 10s request timeout (`nlm.go:239`): the remote's
timeout is non-strict and may answer late, and the TTL is a backstop rather than a competitor to it.
Because expiry now *queries* instead of guessing, a generous TTL costs only delayed cleanup — the
wrong-answer pressure that would otherwise make this timing delicate is gone.

### Layer 5 — Wiring

`finality` declares its own narrow interface rather than depending on `queryservice.QueryService`
(six methods, `queryservice/provider.go:37-46`) when the sweeper needs one:

```go
// TxStatusQuerier resolves the committed status of transactions.
type TxStatusQuerier interface {
    GetTransactionStatuses(txIDs []string) (map[string]int32, error)
}
```

This matches the pattern already established in this package — `GRPCClientProvider` and
`ServiceConfigProvider` (`provider.go:25-48`) are both locally-declared narrow interfaces over other
packages' types — and makes the test double a one-method stub instead of a six-method fake.

Changes:

- `Provider` gains a `queryServiceProvider queryservice.Provider` field.
- `NewListenerManagerProvider` gains a matching third param.
- `newNotifiWithGRPC` takes `channel` in addition to `network` (it currently receives only `network`,
  `provider.go:138`, while `queryservice.Provider.Get` needs both — and `NewManager` already has
  both, `provider.go:91`), resolves the query service, and sets the three new fields.

**`sdk/dig/sdk.go` needs no change** — `dig` resolves the new param from the already-registered
`queryservice.Provider` (`sdk.go:64`).

**No import cycle:** verified `queryservice` does not depend on `finality`
(`go list -deps` returns no match).

## Rejected alternatives

### Rejected: query at registration

Querying in `AddFinalityListener` would satisfy the contract's *"called immediately"* clause
(`committer.go:43`) literally. Rejected because:

- `AddFinalityListener` holds `handlersMu` for its whole body (`nlm.go:216-217`, `defer Unlock`).
  gRPC under that mutex would block every other registration **and** the dispatcher's own
  `handlersMu.Lock()` (`nlm.go:128`), stalling notification delivery for *all* transactions — worse
  than the leak. Querying before the lock avoids that, but:
- per finding #5 the query is **uncancellable and can stall 30s**, so every registration risks a 30s
  hang to fix a case that is rare in normal operation. `IsFinal`'s own `ctx` cancellation
  (`wrappers.go:119`) would not interrupt it.

Sweeper-only accepts that an already-final transaction waits up to one TTL, so *"immediately"* is not
literally met. That clause matters when a transaction committed *before* registration, which for the
in-tree submit-then-wait flow (`wrappers.go:66-122`) is the uncommon case. A pre-lock query can be
added later as an optimisation — ideally after `RemoteQueryService` is fixed to honour caller
contexts.

This area deserves care in review: `nlm.go:44-50` and `nlm.go:242-249` are extensive comments
documenting a *previously fixed* deadlock, with a dedicated `nlm_deadlock_poc_test.go`.

### Rejected: register then verify asynchronously

Keeps registration fast, but the entry exists transiently and resolution is concurrent — more moving
parts to reason about and test, for no benefit over sweeping.

### Rejected: delete-and-retain-for-retry

Remove from `handlers` but hold entries in a side structure for retry. Gets retries without blocking
cleanup, but the side structure needs its own bound, expiry, and eviction policy — reintroducing the
original problem one level down. Not worth it for a backstop that should rarely fire.

## Testing

`core/finality/mock/` already has notifier-client mocks (`notifier_client.go`,
`notifier_grpc_client.go`), so all of this is unit-testable with no network.

- `AddFinalityListener("", l)` → error, no entry, no request sent.
- Rejected txID → listener called with `Invalid` and the server's reason; entry removed.
- **Sweeper, query says committed → `Valid`, not `Unknown`.** This test proves the design: it is the
  case a naive TTL gets wrong.
- Sweeper, tx absent from query result → `Unknown`, entry dropped.
- Sweeper, query returns error → `Unknown`, entry **still dropped** (cleanup unconditional).
- Nil query service → `Unknown`, entry dropped.
- Notification arrives before expiry → listener called exactly **once**, no double-invoke.
- `parseResponse` precedence: same txID in multiple response fields → status beats rejection beats
  timeout.
- Regression: `nlm_deadlock_poc_test.go` still passes; a slow/hanging sweep query does not block the
  dispatcher.

### Test-suite constraints

**Zero-value defaults** (Layer 4) mean existing tests need no *behavioural* changes — the sweeper stays
inert with `listenerTTL == 0`. They do need mechanical edits for the `*handlerEntry` map type; see
Layer 4 for the ~18 affected sites. TTL tests set the duration fields explicitly.

**No clock abstraction exists in this repo** — no `clockwork`, no `Clock` interface anywhere; existing
tests use real `time.After` with short waits (`nlm_test.go:89`). Follow that convention with small
durations (TTL ~50ms, sweep ~10ms) rather than introducing an injectable clock: a new time-mocking
pattern would be a larger change than the fix. Mitigate timing sensitivity by asserting through the
existing `sync.WaitGroup`-based `mockListener` rather than sleeping and polling.

## Out of scope

- **Hardcoded 10s request timeout** (finding #3) — real, but a separate config concern; own issue.
- **Panicking listeners.** A listener that panics kills the goroutine at `nlm.go:145-161` and takes
  the process with it. Pre-existing, not introduced here — but the sweeper reuses that same wrapper,
  so it inherits the exposure. Worth a follow-up issue rather than widening this fix.
- **`RemoteQueryService` ignoring caller contexts** (finding #5) — affects the whole query service,
  not just this path. Would unblock a future registration-time query.
- **`StreamAllTransactions`** (`notify.proto:25`) — unused by FSC.

## Code references

- `platform/fabricx/core/finality/nlm.go:41-42` — `handlers` map and mutex
- `platform/fabricx/core/finality/nlm.go:106-164` — dispatcher; removal at `:134`; ticker goes here
- `platform/fabricx/core/finality/nlm.go:126-139` — collect-under-lock pattern the sweeper follows
- `platform/fabricx/core/finality/nlm.go:144-162` — handler invocation wrapper the sweeper reuses
- `platform/fabricx/core/finality/nlm.go:170-172` — `clear()` on teardown
- `platform/fabricx/core/finality/nlm.go:178-208` — `parseResponse` (Layer 2)
- `platform/fabricx/core/finality/nlm.go:211-262` — `AddFinalityListener`; guard at `:212`, insert at `:225`, timeout at `:239`, send at `:256`
- `platform/fabricx/core/finality/provider.go:25-48` — narrow-interface precedent
- `platform/fabricx/core/finality/provider.go:138-156` — `newNotifiWithGRPC`; wiring target
- `platform/fabricx/core/channel/wrappers.go:66-122` — `IsFinal`, the in-tree consumer
- `platform/fabricx/core/committer/queryservice/query.go:95-132` — `GetTransactionStatuses`
- `platform/fabricx/core/committer/queryservice/query.go:78,115` — `context.Background()` (finding #5)
- `platform/fabricx/core/committer/config/config.go:16` — `DefaultRequestTimeout = 30s`
- `platform/fabricx/sdk/dig/sdk.go:62,64` — DI registrations (no change needed)
- `platform/fabric/committer.go:42-48` — documented public contract
- `platform/common/core/generic/committer/listenermgr.go:54-57` — generic empty-txID guard
- `$GOMODCACHE/github.com/hyperledger/fabric-x-common@v0.2.8/api/committerpb/notify.proto:28-52` — request/response contract
