# Fabric-x Finality Listener Leak Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop `notificationListenerManager.handlers` from growing without bound by making cleanup independent of remote committer liveness, and fix the two deterministic leak paths (empty txID, ignored `RejectedTxIds`).

**Architecture:** Four independent layers on `platform/fabricx/core/finality`. Layer 1 adds an input guard. Layer 2 teaches `parseResponse` to read the `rejected_tx_ids` response field and carry the server's reason string. Layer 3 adds a per-entry deadline swept from the *existing* dispatcher goroutine, which deletes entries **before** querying the committer for their true status — so cleanup never depends on a network call. Layer 4 wires the query service in via DI.

**Tech Stack:** Go 1.x, gRPC (`github.com/hyperledger/fabric-x-common/api/committerpb`), `dig` DI, `counterfeiter` mocks, `testify` (`require`/`assert`/`EventuallyWithT`), `errgroup`.

**Spec:** [`docs/superpowers/specs/2026-08-03-fabricx-finality-listener-leak-design.md`](../specs/2026-08-03-fabricx-finality-listener-leak-design.md)
**Issue:** [#1626](https://github.com/hyperledger-labs/fabric-smart-client/issues/1626)
**Branch:** `fix/1626-fabricx-finality-listener-leak` (already exists, spec committed)

## Global Constraints

- **Errors:** use `github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors` (`New`/`Errorf`/`Wrap`/`Wrapf`). **Never** `fmt.Errorf`.
- **Logging:** the package-level `logger` in `nlm.go:27`. Use `logger.Debugf`/`Warnf`/`Errorf`.
- **Commits:** sign off every commit — `git commit -s` (DCO enforced). Rebase, never merge.
- **No new dependencies.** In particular **do not add a clock library** (`clockwork` etc.) — none exists in this repo. Tests use real durations.
- **Test durations:** reuse the existing constants in `nlm_test.go:32-36` (`tick = 10ms`, `timeout = 1s`, `shortWait = 100ms`). New TTL tests use `testTTL = 50 * time.Millisecond` and `testSweep = 10 * time.Millisecond`.
- **`nlm_deadlock_poc_test.go` must keep passing** — it guards a previously fixed deadlock.
- **Gates:** `make unit-tests` and `make checks` must pass before the final commit of every task.
- **Run tests with `-race`.** `make unit-tests` already does; when running focused tests add it explicitly.
- Do **not** change the `ListenerManager` interface (`provider.go:33-36`) — no caller churn.
- Do **not** touch the hardcoded 10s request timeout (`nlm.go:239`) — explicitly out of scope.

## File Structure

| File | Change | Responsibility |
|------|--------|----------------|
| `platform/fabricx/core/finality/nlm.go` | Modify | All four layers: guard, `parseResponse`, `handlerEntry`, sweeper |
| `platform/fabricx/core/finality/provider.go` | Modify | `TxStatusQuerier` interface + DI wiring |
| `platform/fabricx/core/finality/nlm_test.go` | Modify | Migrate 12 map sites; extend `mockListener`; new tests |
| `platform/fabricx/core/finality/nlm_deadlock_poc_test.go` | Modify | One map-shape line (`:84` is an existence check — verify only) |
| `platform/fabricx/core/finality/mock/tx_status_querier.go` | Create | Hand-written one-method stub (not counterfeiter) |

Everything lands in one package. No new files beyond the test stub.

## Task Order Rationale

Task 1 (guard) is independent and ships value immediately. Task 2 (map shape) is pure mechanical refactor with **zero behaviour change** — isolating it means the churn is reviewable separately from logic. Tasks 3–5 add behaviour on the migrated shape. Task 6 wires DI last, so every prior task is testable without touching `provider.go`.

---

### Task 1: Reject empty txID

**Files:**
- Modify: `platform/fabricx/core/finality/nlm.go:211-214`
- Test: `platform/fabricx/core/finality/nlm_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: no new symbols. `AddFinalityListener` gains an error return path for `txID == ""`.

- [ ] **Step 1: Write the failing test**

Add this subtest inside the existing `TestNotificationListenerManager` function in `nlm_test.go`, immediately after the `AddFinalityListener_Nil_Listener_Fails` subtest (which ends at `nlm_test.go:421`):

```go
	t.Run("AddFinalityListener_Empty_TxID_Fails", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		ml := &mockListener{}
		err := nlm.AddFinalityListener("", ml)

		require.Error(t, err)
		require.EqualError(t, err, "tx id must be not empty")

		// no entry must be created for the empty key
		nlm.handlersMu.RLock()
		_, exists := nlm.handlers[""]
		nlm.handlersMu.RUnlock()
		require.False(t, exists, "No handler entry should be created for an empty txID")

		// and no subscription must be sent
		time.Sleep(shortWait)
		require.Equal(t, 0, fakeStream.SendCallCount(), "Empty txID must not trigger a Send")
	})
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -race -run 'TestNotificationListenerManager/AddFinalityListener_Empty_TxID_Fails' ./platform/fabricx/core/finality/ -v
```

Expected: FAIL. `require.Error` fails because `AddFinalityListener` currently returns `nil` for an empty txID.

- [ ] **Step 3: Add the guard**

In `nlm.go`, `AddFinalityListener` currently begins:

```go
func (n *notificationListenerManager) AddFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error {
	if listener == nil {
		return errors.New("listener nil")
	}
```

Add the txID guard immediately after the nil check:

```go
func (n *notificationListenerManager) AddFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error {
	if listener == nil {
		return errors.New("listener nil")
	}
	// An empty txID can never be matched by any committer notification, so the
	// map entry would be unremovable. Matches the generic driver's guard in
	// platform/common/core/generic/committer/listenermgr.go.
	if len(txID) == 0 {
		return errors.New("tx id must be not empty")
	}
```

Note: the message string must be exactly `tx id must be not empty` to match the generic driver (`platform/common/core/generic/committer/listenermgr.go:56`).

- [ ] **Step 4: Run test to verify it passes**

```bash
go test -race -run 'TestNotificationListenerManager/AddFinalityListener_Empty_TxID_Fails' ./platform/fabricx/core/finality/ -v
```

Expected: PASS.

- [ ] **Step 5: Run the full package plus gates**

```bash
go test -race ./platform/fabricx/core/finality/
make checks
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add platform/fabricx/core/finality/nlm.go platform/fabricx/core/finality/nlm_test.go
git commit -s -m "fix(fabricx): reject empty txID in AddFinalityListener

An empty txID creates a handlers entry that no committer notification can
ever match, so it is never removed. The generic driver already guards this;
Fabric-x accepted it silently.

Refs #1626"
```

---

### Task 2: Migrate `handlers` to `*handlerEntry` (mechanical, no behaviour change)

**Files:**
- Modify: `platform/fabricx/core/finality/nlm.go:41`, `:126-139`, `:219-230`, `:270-302`
- Modify: `platform/fabricx/core/finality/provider.go:151`
- Test: `platform/fabricx/core/finality/nlm_test.go` (12 sites + helper)

**Interfaces:**
- Consumes: Task 1's guard (unchanged).
- Produces:
  - `type handlerEntry struct { listeners []fabric.FinalityListener; expiresAt time.Time }`
  - `handlers map[driver.TxID]*handlerEntry` field on `notificationListenerManager`
  - test helper `seedHandlers(nlm *notificationListenerManager, txID string, listeners ...fabric.FinalityListener)`

**Critical:** `expiresAt` is *added but unused* in this task. Zero behaviour change — this task exists purely to isolate the mechanical churn from the logic changes in Tasks 3-5. Do not add sweeping here.

**Exactly 12 test sites break** (the `_, exists :=` existence checks compile unchanged because they discard the value):

| Kind | Sites |
|------|-------|
| Seed (assign a slice) | `nlm_test.go:200, 248, 648, 712, 788, 789` |
| Element access (`Len`/index/`Contains`) | `nlm_test.go:339, 380, 526, 534, 573, 582` |

`nlm_deadlock_poc_test.go:84` is `_, exists :=` — **verify it compiles, do not edit**.

- [ ] **Step 1: Add the type and change the field**

In `nlm.go`, add above `type notificationListenerManager struct`:

```go
// handlerEntry holds the listeners registered for one transaction, plus the
// deadline after which the entry is swept locally. expiresAt is zero when
// local expiry is disabled (listenerTTL == 0).
type handlerEntry struct {
	listeners []fabric.FinalityListener
	expiresAt time.Time
}
```

Change the field at `nlm.go:41` from:

```go
	handlers   map[driver.TxID][]fabric.FinalityListener
```

to:

```go
	handlers   map[driver.TxID]*handlerEntry
```

- [ ] **Step 2: Update the three production readers/writers**

**2a. Dispatcher** (`nlm.go:128-139`) — change the inner loop body:

```go
			n.handlersMu.Lock()
			for txID, v := range res {
				entry, ok := n.handlers[txID]
				if !ok {
					continue
				}
				delete(n.handlers, txID)
				for _, h := range entry.listeners {
					calls = append(calls, handlerCall{handler: h, txID: txID, status: v})
				}
			}
			n.handlersMu.Unlock()
```

**2b. `AddFinalityListener`** (`nlm.go:219-230`) — replace:

```go
	handlers := n.handlers[txID]
	if slices.Contains(handlers, listener) {
		logger.Warnf("The exact same listener is already registered for txID=%v. Skipping.", txID)
		// Do not register the same instance twice
		return nil
	}
	n.handlers[txID] = append(handlers, listener)

	if len(handlers) > 0 {
		logger.Debugf("Additional listener registered for txID=%v. Request already sent.", txID)
		return nil
	}
```

with:

```go
	entry, existed := n.handlers[txID]
	if existed {
		if slices.Contains(entry.listeners, listener) {
			logger.Warnf("The exact same listener is already registered for txID=%v. Skipping.", txID)
			// Do not register the same instance twice
			return nil
		}
		entry.listeners = append(entry.listeners, listener)
		logger.Debugf("Additional listener registered for txID=%v. Request already sent.", txID)
		return nil
	}

	n.handlers[txID] = &handlerEntry{listeners: []fabric.FinalityListener{listener}}
```

Note the restructure: previously `len(handlers) > 0` distinguished first-vs-subsequent registration. Now `existed` does it directly, which is clearer and avoids re-reading the slice length.

**2c. `RemoveFinalityListener`** (`nlm.go:270-302`) — replace the body after the nil check:

```go
	n.handlersMu.Lock()
	defer n.handlersMu.Unlock()

	entry, ok := n.handlers[txID]
	if !ok || len(entry.listeners) == 0 {
		// no handlers registered for this txID, nothing to remove
		logger.Debugf("RemoveFinalityListener called for unknown txID: %s", txID)
		return nil
	}

	initialLength := len(entry.listeners)

	newHandlers := slices.DeleteFunc(entry.listeners, func(h fabric.FinalityListener) bool {
		return h == listener
	})

	if len(newHandlers) == initialLength {
		// if the length is the same, no listener was removed.
		logger.Warnf("Listener not found for txID=%s, cannot remove.", txID)
		return nil
	}

	// check if the list of handlers is now empty
	if len(newHandlers) == 0 {
		// this was the last listener. Clean up our local map entry.
		logger.Debugf("Last finality listener removed for txID=%s.", txID)
		delete(n.handlers, txID)
	} else {
		entry.listeners = newHandlers
		logger.Debugf("Removed listener for txID=%s. %d listeners remaining.", txID, len(newHandlers))
	}

	return nil
```

- [ ] **Step 3: Update the constructor**

In `provider.go:151`, change:

```go
		handlers:       make(map[string][]fabric.FinalityListener),   // Map: txID -> list of listeners
```

to:

```go
		handlers:       make(map[driver.TxID]*handlerEntry), // Map: txID -> listeners + local expiry deadline
```

Verify `driver` is already imported in `provider.go` (it is — `provider.go:18`). If `fabric` becomes unused there, remove the import; run `make checks` to confirm.

- [ ] **Step 4: Add the test helper**

In `nlm_test.go`, add after `setupTest` (which ends at `nlm_test.go:116`):

```go
// seedHandlers injects listeners directly into the handlers map, bypassing
// AddFinalityListener, to isolate dispatch/sweep logic in tests. Localises the
// map's internal shape so future changes touch one place.
func seedHandlers(nlm *notificationListenerManager, txID string, listeners ...fabric.FinalityListener) {
	nlm.handlersMu.Lock()
	defer nlm.handlersMu.Unlock()
	nlm.handlers[txID] = &handlerEntry{listeners: listeners}
}

// listenersFor returns a copy of the listeners registered for txID, plus whether
// the entry exists.
func listenersFor(nlm *notificationListenerManager, txID string) ([]fabric.FinalityListener, bool) {
	nlm.handlersMu.RLock()
	defer nlm.handlersMu.RUnlock()
	entry, ok := nlm.handlers[txID]
	if !ok {
		return nil, false
	}
	return slices.Clone(entry.listeners), true
}
```

Add `"slices"` to the `nlm_test.go` import block (`nlm_test.go:9-26`).

- [ ] **Step 5: Migrate the 6 seed sites**

Replace each direct assignment with a `seedHandlers` call. The six sites and their replacements:

```go
// nlm_test.go:200 (inside the table-driven Receive_And_Dispatch_HappyPath loop)
seedHandlers(nlm, tc.txID, ml)

// nlm_test.go:248
seedHandlers(nlm, targetTxID, ml)

// nlm_test.go:648
seedHandlers(nlm, targetTxID, slowListener)

// nlm_test.go:712
seedHandlers(nlm, targetTxID, fastML, slowML, stuckListener)

// nlm_test.go:788 and :789
seedHandlers(nlm, leakyTxID, leakyListener)
seedHandlers(nlm, normalTxID, normalML)
```

Note `seedHandlers` takes its own lock, so remove any surrounding `nlm.handlersMu.Lock()`/`Unlock()` that existed *solely* to guard these assignments. Sites `:200`, `:248`, `:648`, `:712`, `:788-789` are pre-`runManager` setup and are currently unlocked — check each before deleting anything.

- [ ] **Step 6: Migrate the 6 element-access sites**

```go
// nlm_test.go:338-343  (Duplicate_Is_Rejected)
handlers, exists := listenersFor(nlm, targetTxID)
require.True(t, exists, "Handler list should exist after first registration")
require.Len(t, handlers, 1, "There should be exactly ONE registered handler (the duplicate was rejected)")
require.Equal(t, ml, handlers[0], "The registered handler must be the original instance (ml)")

// nlm_test.go:379-384  (Multiple_Unique_Are_Allowed)
handlers, exists := listenersFor(nlm, targetTxID)
require.True(t, exists, "Handler list should exist")
require.Len(t, handlers, 2, "There should be exactly TWO registered handlers")

// nlm_test.go:525-527  (Remove one of two)
handlers, exists := listenersFor(nlm, targetTxID)
require.True(t, exists, "Setup: entry should exist")
require.Len(t, handlers, 2, "Setup: Expected 2 listeners")

// nlm_test.go:533-539
handlers, exists = listenersFor(nlm, targetTxID)
require.True(t, exists, "Map entry should still exist")
require.Len(t, handlers, 1, "Expected 1 listener remaining (ml2)")
require.Equal(t, ml2, handlers[0], "The remaining listener must be ml2")

// nlm_test.go:572-574  (Remove_NonExistent_Listener setup)
handlers, exists := listenersFor(nlm, targetTxID)
require.True(t, exists, "Setup: entry should exist")
require.Len(t, handlers, 2, "Setup: Expected 2 listeners")

// nlm_test.go:581-586
handlers, exists = listenersFor(nlm, targetTxID)
require.True(t, exists, "Map entry should still exist")
require.Len(t, handlers, 2, "The number of handlers should not change")
```

In each case delete the now-redundant `nlm.handlersMu.RLock()`/`RUnlock()` pair that wrapped the original access — `listenersFor` locks internally. Mind `:=` vs `=` where `handlers`/`exists` are already declared in scope (sites `:533` and `:581` reuse existing variables).

`nlm_test.go:611` (`require.Empty(t, nlm.handlers, ...)`) works unchanged — it asserts on the map, not an element.

- [ ] **Step 7: Verify the whole package compiles and passes with zero behaviour change**

```bash
go build ./platform/fabricx/core/finality/
go test -race ./platform/fabricx/core/finality/ -count=1
```

Expected: PASS, including `TestAddFinalityListenerRecoversAfterStreamFailure` in `nlm_deadlock_poc_test.go`. **No test should have needed a semantic change** — if any assertion had to be weakened or an expectation altered, stop: the refactor changed behaviour and that is a bug.

- [ ] **Step 8: Run gates**

```bash
make unit-tests
make checks
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add platform/fabricx/core/finality/
git commit -s -m "refactor(fabricx): hold finality listeners in handlerEntry

Replaces map[TxID][]FinalityListener with map[TxID]*handlerEntry so a local
expiry deadline can be attached in a follow-up. expiresAt is unused here.

Pure refactor, no behaviour change. Test map access moves behind seedHandlers
and listenersFor helpers.

Refs #1626"
```

---

### Task 3: Carry a status message and handle `RejectedTxIds`

**Files:**
- Modify: `platform/fabricx/core/finality/nlm.go:107-111`, `:121`, `:126-139`, `:151`, `:178-208`
- Test: `platform/fabricx/core/finality/nlm_test.go`

**Interfaces:**
- Consumes: `handlerEntry` from Task 2.
- Produces:
  - `type txOutcome struct { status int; message string }`
  - `parseResponse(resp *committerpb.NotificationResponse) map[string]txOutcome` (signature change)
  - `mockListener` gains an `errMsg string` field and `getOutcome() (string, int, string)`.

- [ ] **Step 1: Extend `mockListener` to capture `errMsg`**

`mockListener.OnStatus` currently **discards** `errMsg` (`nlm_test.go:46-52`), so no test can assert on a reason. Change it to:

```go
// mockListener is a helper to verify callbacks
type mockListener struct {
	txID   string
	status int
	errMsg string
	wg     sync.WaitGroup
	lock   sync.RWMutex
}

func (m *mockListener) OnStatus(ctx context.Context, txID string, status int, errMsg string) {
	m.lock.Lock()
	m.txID = txID
	m.status = status
	m.errMsg = errMsg
	m.lock.Unlock()
	m.wg.Done()
}

// getStatus is a helper to safely read the state for use in EventuallyWithT
func (m *mockListener) getStatus() (string, int) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.txID, m.status
}

// getOutcome additionally returns the status message.
func (m *mockListener) getOutcome() (string, int, string) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.txID, m.status, m.errMsg
}
```

`getStatus` is kept so existing call sites (`nlm_test.go:267`, `:222` etc.) stay unchanged.

- [ ] **Step 2: Write the failing tests**

Add two subtests inside `TestNotificationListenerManager`, after the `Receive_And_Dispatch_Handles_Timeout` subtest (ends `nlm_test.go:276`):

```go
	t.Run("Receive_And_Dispatch_Handles_Rejection", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_rejected"
		const reason = "namespace policy not satisfied"
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()
		ml := &mockListener{}
		ml.wg.Add(1)
		seedHandlers(nlm, targetTxID, ml)

		resp := &committerpb.NotificationResponse{
			RejectedTxIds: &committerpb.RejectedTxIds{
				TxIds:  []string{targetTxID},
				Reason: reason,
			},
		}

		var sent atomic.Bool
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			if !sent.Swap(true) {
				return resp, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status, errMsg := ml.getOutcome()
			assert.Equal(collect, targetTxID, txID)
			assert.Equal(collect, fdriver.Invalid, status, "A rejected tx is definitively Invalid, not Unknown")
			assert.Equal(collect, reason, errMsg, "The committer's rejection reason must reach the listener")
		}, timeout, tick, "timeout waiting for OnStatus from rejection response")

		_, exists := listenersFor(nlm, targetTxID)
		require.False(t, exists, "Handler should be removed after a rejection")
	})

	t.Run("ParseResponse_Precedence", func(t *testing.T) {
		t.Parallel()
		const txID = "tx_precedence"

		t.Run("status beats rejection and timeout", func(t *testing.T) {
			t.Parallel()
			out := parseResponse(&committerpb.NotificationResponse{
				TimeoutTxIds:  []string{txID},
				RejectedTxIds: &committerpb.RejectedTxIds{TxIds: []string{txID}, Reason: "rejected"},
				TxStatusEvents: []*committerpb.TxStatus{{
					Ref:    &committerpb.TxRef{TxId: txID},
					Status: committerpb.Status_COMMITTED,
				}},
			})
			require.Equal(t, fdriver.Valid, out[txID].status)
			require.Empty(t, out[txID].message, "A committed status carries no message")
		})

		t.Run("rejection beats timeout", func(t *testing.T) {
			t.Parallel()
			out := parseResponse(&committerpb.NotificationResponse{
				TimeoutTxIds:  []string{txID},
				RejectedTxIds: &committerpb.RejectedTxIds{TxIds: []string{txID}, Reason: "rejected"},
			})
			require.Equal(t, fdriver.Invalid, out[txID].status)
			require.Equal(t, "rejected", out[txID].message)
		})

		t.Run("nil rejected field is safe", func(t *testing.T) {
			t.Parallel()
			out := parseResponse(&committerpb.NotificationResponse{
				TimeoutTxIds: []string{txID},
			})
			require.Equal(t, fdriver.Unknown, out[txID].status)
			require.Empty(t, out[txID].message)
		})
	})
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test -race -run 'TestNotificationListenerManager/(Receive_And_Dispatch_Handles_Rejection|ParseResponse_Precedence)' ./platform/fabricx/core/finality/ -v
```

Expected: compile FAIL — `out[txID].status` is invalid because `parseResponse` returns `map[string]int`.

- [ ] **Step 4: Change `parseResponse`**

Replace `nlm.go:178-208` entirely:

```go
// txOutcome is the resolved status for one transaction, plus an optional
// human-readable message (currently only set for committer rejections).
type txOutcome struct {
	status  int
	message string
}

// parseResponse flattens a NotificationResponse into per-txID outcomes.
//
// Precedence, weakest to strongest — a txID appearing in several fields takes
// the strongest: timeout (Unknown) < rejection (Invalid) < status event. A
// definitive commit status always wins, and a rejection always beats a mere
// timeout. Keep this ordering if you add another response field.
func parseResponse(resp *committerpb.NotificationResponse) map[string]txOutcome {
	res := make(map[string]txOutcome)

	// weakest: timeouts
	for _, txID := range resp.GetTimeoutTxIds() {
		res[txID] = txOutcome{status: fdriver.Unknown}
	}

	// stronger: rejections. The committer will never process these, so they are
	// definitively Invalid rather than Unknown. One reason applies to the whole
	// batch. GetRejectedTxIds() is nil-safe on a nil receiver.
	rejected := resp.GetRejectedTxIds()
	for _, txID := range rejected.GetTxIds() {
		res[txID] = txOutcome{status: fdriver.Invalid, message: rejected.GetReason()}
		logger.Debugf("transaction [%s] rejected by committer: %s", txID, rejected.GetReason())
	}

	// strongest: actual status events
	for _, r := range resp.GetTxStatusEvents() {
		txID := r.GetRef().GetTxId()
		status := r.GetStatus()

		logger.Debugf("transaction [%s] status [%s]", txID, status)

		res[txID] = txOutcome{status: statusFromCommitter(status)}
	}

	return res
}

// statusFromCommitter maps a committer status onto an fdriver validation code.
// Shared by parseResponse and the expiry sweeper so both interpret a committer
// status identically.
func statusFromCommitter(status committerpb.Status) int {
	switch status {
	case committerpb.Status_COMMITTED:
		return fdriver.Valid
	case committerpb.Status_STATUS_UNSPECIFIED:
		return fdriver.Unknown
	default:
		return fdriver.Invalid
	}
}
```

Note this extracts the existing `switch` from `parseResponse` into `statusFromCommitter` so Task 4's
sweeper can reuse it. `fdriver.ValidationCode` is an alias for `int`
(`platform/fabric/driver/committer.go:18`), so an `int` return type is correct and matches what
`parseResponse` already produced.

- [ ] **Step 5: Thread the message through the dispatcher**

In `nlm.go`, extend `handlerCall` (`nlm.go:107-111`):

```go
		type handlerCall struct {
			handler fabric.FinalityListener
			txID    string
			status  int
			message string
		}
```

In the collect loop (`nlm.go:129-138`), carry the message:

```go
			n.handlersMu.Lock()
			for txID, outcome := range res {
				entry, ok := n.handlers[txID]
				if !ok {
					continue
				}
				delete(n.handlers, txID)
				for _, h := range entry.listeners {
					calls = append(calls, handlerCall{
						handler: h,
						txID:    txID,
						status:  outcome.status,
						message: outcome.message,
					})
				}
			}
			n.handlersMu.Unlock()
```

And at the `OnStatus` call (`nlm.go:151`), replace the hardcoded `""`:

```go
						c.handler.OnStatus(timeoutCtx, c.txID, c.status, c.message)
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test -race -run 'TestNotificationListenerManager/(Receive_And_Dispatch_Handles_Rejection|ParseResponse_Precedence)' ./platform/fabricx/core/finality/ -v
go test -race ./platform/fabricx/core/finality/ -count=1
```

Expected: all PASS.

- [ ] **Step 7: Run gates**

```bash
make unit-tests
make checks
```

- [ ] **Step 8: Commit**

```bash
git add platform/fabricx/core/finality/
git commit -s -m "fix(fabricx): handle RejectedTxIds from the committer

parseResponse read only tx_status_events and timeout_tx_ids, ignoring
rejected_tx_ids entirely. A rejection the committer explicitly reported left
the handlers entry in place forever -- a leak needing no server fault.

Rejections now resolve to Invalid (definitive, not Unknown) and the server's
reason reaches OnStatus, which previously always received an empty string.
Field precedence is now explicit: timeout < rejection < status event.

Refs #1626"
```

---

### Task 4: Query-backed expiry sweeper

**Files:**
- Modify: `platform/fabricx/core/finality/nlm.go:29-33`, `:35-51`, `:106-164`, `:219-230`
- Create: `platform/fabricx/core/finality/mock/tx_status_querier.go`
- Test: `platform/fabricx/core/finality/nlm_test.go`

**Interfaces:**
- Consumes: `handlerEntry`/`expiresAt` (Task 2), `txOutcome` (Task 3).
- Produces:
  - `type TxStatusQuerier interface { GetTransactionStatuses(txIDs []string) (map[string]int32, error) }`
  - fields `queryService TxStatusQuerier`, `listenerTTL time.Duration`, `sweepInterval time.Duration`
  - `const DefaultListenerTTL = 2 * time.Minute`, `const DefaultSweepInterval = 30 * time.Second`
  - method `(*notificationListenerManager).sweepExpired(ctx context.Context)`
  - test stub `mock.TxStatusQuerier`

- [ ] **Step 1: Create the query-service test stub**

Hand-written rather than counterfeiter — it is one method, and this keeps `go generate` output untouched.

Create `platform/fabricx/core/finality/mock/tx_status_querier.go`:

```go
/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mock

import "sync"

// TxStatusQuerier is a hand-written stub for finality.TxStatusQuerier.
type TxStatusQuerier struct {
	lock sync.Mutex
	// GetTransactionStatusesStub, when set, fully controls the return values.
	GetTransactionStatusesStub func(txIDs []string) (map[string]int32, error)
	calls                      [][]string
}

func (m *TxStatusQuerier) GetTransactionStatuses(txIDs []string) (map[string]int32, error) {
	m.lock.Lock()
	m.calls = append(m.calls, txIDs)
	stub := m.GetTransactionStatusesStub
	m.lock.Unlock()

	if stub != nil {
		return stub(txIDs)
	}
	return map[string]int32{}, nil
}

// CallCount returns how many times GetTransactionStatuses was invoked.
func (m *TxStatusQuerier) CallCount() int {
	m.lock.Lock()
	defer m.lock.Unlock()
	return len(m.calls)
}

// ArgsForCall returns the txIDs passed to the i-th invocation.
func (m *TxStatusQuerier) ArgsForCall(i int) []string {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.calls[i]
}
```

- [ ] **Step 2: Write the failing tests**

Add a new top-level test function at the end of `nlm_test.go`:

```go
const (
	testTTL   = 50 * time.Millisecond
	testSweep = 10 * time.Millisecond
)

// setupSweepTest builds a manager with local expiry enabled and a blocking Recv,
// so nothing but the sweeper touches the handlers map.
func setupSweepTest(tb testing.TB, qs TxStatusQuerier) (*notificationListenerManager, *mock.Notifier_OpenNotificationStreamClient) {
	tb.Helper()
	nlm, fakeStream := setupTest(tb)
	nlm.listenerTTL = testTTL
	nlm.sweepInterval = testSweep
	nlm.queryService = qs
	return nlm, fakeStream
}

func TestSweepExpired(t *testing.T) {
	t.Parallel()

	t.Run("Query_Says_Committed_Reports_Valid", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_sweep_committed"
		qs := &mock.TxStatusQuerier{
			GetTransactionStatusesStub: func(txIDs []string) (map[string]int32, error) {
				return map[string]int32{targetTxID: int32(committerpb.Status_COMMITTED)}, nil
			},
		}
		nlm, fakeStream := setupSweepTest(t, qs)
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		ml := &mockListener{}
		ml.wg.Add(1)
		seedHandlers(nlm, targetTxID, ml)
		expireNow(nlm, targetTxID)

		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status := ml.getStatus()
			assert.Equal(collect, targetTxID, txID)
			assert.Equal(collect, fdriver.Valid, status,
				"expiry must report the queried status, not a blind Unknown")
		}, timeout, tick, "timeout waiting for sweeper OnStatus")

		_, exists := listenersFor(nlm, targetTxID)
		require.False(t, exists, "Expired entry must be removed")
	})

	t.Run("Tx_Absent_From_Query_Reports_Unknown", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_sweep_absent"
		qs := &mock.TxStatusQuerier{
			GetTransactionStatusesStub: func(txIDs []string) (map[string]int32, error) {
				return map[string]int32{}, nil // absent => not final
			},
		}
		nlm, fakeStream := setupSweepTest(t, qs)
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		ml := &mockListener{}
		ml.wg.Add(1)
		seedHandlers(nlm, targetTxID, ml)
		expireNow(nlm, targetTxID)

		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			_, status := ml.getStatus()
			assert.Equal(collect, fdriver.Unknown, status)
		}, timeout, tick, "timeout waiting for Unknown from sweeper")

		_, exists := listenersFor(nlm, targetTxID)
		require.False(t, exists, "Expired entry must be removed")
	})

	t.Run("Query_Error_Still_Removes_Entry", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_sweep_query_error"
		qs := &mock.TxStatusQuerier{
			GetTransactionStatusesStub: func(txIDs []string) (map[string]int32, error) {
				return nil, errors.New("query service unavailable")
			},
		}
		nlm, fakeStream := setupSweepTest(t, qs)
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		ml := &mockListener{}
		ml.wg.Add(1)
		seedHandlers(nlm, targetTxID, ml)
		expireNow(nlm, targetTxID)

		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			_, status := ml.getStatus()
			assert.Equal(collect, fdriver.Unknown, status)
		}, timeout, tick, "a failed query must still notify with Unknown")

		_, exists := listenersFor(nlm, targetTxID)
		require.False(t, exists,
			"cleanup must be unconditional: a failed query must not resurrect the leak")
	})

	t.Run("Nil_Query_Service_Reports_Unknown", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_sweep_nil_qs"
		nlm, fakeStream := setupSweepTest(t, nil)
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		ml := &mockListener{}
		ml.wg.Add(1)
		seedHandlers(nlm, targetTxID, ml)
		expireNow(nlm, targetTxID)

		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			_, status := ml.getStatus()
			assert.Equal(collect, fdriver.Unknown, status)
		}, timeout, tick, "nil query service must degrade to Unknown")

		_, exists := listenersFor(nlm, targetTxID)
		require.False(t, exists, "Expired entry must be removed")
	})

	t.Run("Unexpired_Entry_Survives", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_sweep_not_yet"
		qs := &mock.TxStatusQuerier{}
		nlm, fakeStream := setupSweepTest(t, qs)
		nlm.listenerTTL = time.Hour // far in the future
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		ml := &mockListener{} // no wg.Add: OnStatus must NOT be called
		seedHandlers(nlm, targetTxID, ml)
		setExpiry(nlm, targetTxID, time.Now().Add(time.Hour))

		runManager(t, nlm)
		time.Sleep(shortWait) // several sweep intervals

		_, exists := listenersFor(nlm, targetTxID)
		require.True(t, exists, "An unexpired entry must not be swept")
		require.Equal(t, 0, qs.CallCount(), "No query should be issued when nothing is expired")
	})

	t.Run("Expiry_Disabled_When_TTL_Zero", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_sweep_disabled"
		qs := &mock.TxStatusQuerier{}
		nlm, fakeStream := setupTest(t) // no TTL fields set => disabled
		nlm.queryService = qs
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		ml := &mockListener{} // no wg.Add: OnStatus must NOT be called
		seedHandlers(nlm, targetTxID, ml)
		expireNow(nlm, targetTxID) // even with a past deadline

		runManager(t, nlm)
		time.Sleep(shortWait)

		_, exists := listenersFor(nlm, targetTxID)
		require.True(t, exists, "listenerTTL==0 must disable expiry entirely")
		require.Equal(t, 0, qs.CallCount(), "No query when expiry is disabled")
	})

	t.Run("Notification_Wins_No_Double_Invoke", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_sweep_no_double"
		qs := &mock.TxStatusQuerier{}
		nlm, fakeStream := setupSweepTest(t, qs)
		ctx := t.Context()

		resp := &committerpb.NotificationResponse{
			TxStatusEvents: []*committerpb.TxStatus{{
				Ref:    &committerpb.TxRef{TxId: targetTxID},
				Status: committerpb.Status_COMMITTED,
			}},
		}
		var sent atomic.Bool
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			if !sent.Swap(true) {
				return resp, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}

		// wg.Add(1) means a second OnStatus call panics with "negative WaitGroup
		// counter" -- which is exactly the double-invoke we are guarding against.
		ml := &mockListener{}
		ml.wg.Add(1)
		seedHandlers(nlm, targetTxID, ml)

		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			_, status := ml.getStatus()
			assert.Equal(collect, fdriver.Valid, status)
		}, timeout, tick, "notification should deliver Valid")

		// let several sweep intervals elapse past the TTL
		time.Sleep(4 * testTTL)

		_, exists := listenersFor(nlm, targetTxID)
		require.False(t, exists, "entry removed by the notification")
	})
}
```

Add these two helpers next to `seedHandlers` in `nlm_test.go`:

```go
// setExpiry overrides an entry's local expiry deadline.
func setExpiry(nlm *notificationListenerManager, txID string, at time.Time) {
	nlm.handlersMu.Lock()
	defer nlm.handlersMu.Unlock()
	if entry, ok := nlm.handlers[txID]; ok {
		entry.expiresAt = at
	}
}

// expireNow backdates an entry so the next sweep collects it.
func expireNow(nlm *notificationListenerManager, txID string) {
	setExpiry(nlm, txID, time.Now().Add(-time.Second))
}
```

`setupSweepTest` references `TxStatusQuerier` from the `finality` package (Step 4 defines it) — the parameter type is `TxStatusQuerier`, satisfied by `*mock.TxStatusQuerier` and by `nil`.

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test -race -run TestSweepExpired ./platform/fabricx/core/finality/ -v
```

Expected: compile FAIL — `nlm.listenerTTL`, `nlm.sweepInterval`, `nlm.queryService`, and `TxStatusQuerier` do not exist yet.

- [ ] **Step 4: Add the interface, constants, and fields**

In `nlm.go`, after `DefaultHandlerTimeout` (`nlm.go:33`):

```go
// DefaultListenerTTL is how long a finality listener may sit unresolved before
// the local sweeper settles it. Deliberately far longer than the 10s request
// timeout in AddFinalityListener: the committer's timeout is documented
// non-strict (it may notify later), so this is a backstop for genuine silence
// rather than a competitor to the remote deadline. Because expiry queries the
// committer for the real status instead of guessing, a generous value costs
// only delayed cleanup.
const DefaultListenerTTL = 2 * time.Minute

// DefaultSweepInterval is how often the dispatcher checks for expired entries.
// An entry's worst-case lifetime is DefaultListenerTTL + DefaultSweepInterval.
const DefaultSweepInterval = 30 * time.Second

// TxStatusQuerier resolves the committed status of transactions. Narrower than
// queryservice.QueryService (six methods) because the sweeper needs exactly one;
// mirrors the locally-declared interfaces in provider.go.
type TxStatusQuerier interface {
	GetTransactionStatuses(txIDs []string) (map[string]int32, error)
}
```

Add three fields to `notificationListenerManager` (after `handlerTimeout`, `nlm.go:39`):

```go
	// queryService resolves the true status of expiring entries. When nil, the
	// sweeper reports Unknown instead of querying.
	queryService TxStatusQuerier
	// listenerTTL bounds how long an entry may stay unresolved. Zero disables
	// local expiry entirely, which is what the test setup relies on and what a
	// missing wire-up degrades to.
	listenerTTL time.Duration
	// sweepInterval is the sweep tick period. Ignored when listenerTTL is zero.
	sweepInterval time.Duration
```

- [ ] **Step 5: Stamp the deadline on registration**

In `AddFinalityListener`, the new-entry branch from Task 2 becomes:

```go
	n.handlers[txID] = &handlerEntry{
		listeners: []fabric.FinalityListener{listener},
		expiresAt: n.expiryFor(time.Now()),
	}
```

And add the helper next to it:

```go
// expiryFor returns the local expiry deadline for an entry created at now, or
// the zero time when local expiry is disabled.
func (n *notificationListenerManager) expiryFor(now time.Time) time.Time {
	if n.listenerTTL <= 0 {
		return time.Time{}
	}
	return now.Add(n.listenerTTL)
}
```

- [ ] **Step 6: Add `sweepExpired`**

Add after the `listen` method in `nlm.go`:

```go
// sweepExpired settles entries whose local deadline has passed.
//
// Entries are deleted from the map BEFORE the status query, deliberately:
// cleanup must never depend on a network call. The query service and the
// notification stream both talk to the same committer, so the fault that lost
// the notification is correlated with the query failing -- if removal waited on
// a successful query, the sweeper would be useless in exactly the situation it
// exists for. A failed query therefore degrades to Unknown, not to a retained
// entry.
func (n *notificationListenerManager) sweepExpired(ctx context.Context) {
	if n.listenerTTL <= 0 {
		return
	}

	now := time.Now()

	// Phase 1: collect and delete under the lock. No I/O here.
	type expired struct {
		txID      string
		listeners []fabric.FinalityListener
	}
	var batch []expired

	n.handlersMu.Lock()
	for txID, entry := range n.handlers {
		if entry.expiresAt.IsZero() || entry.expiresAt.After(now) {
			continue
		}
		batch = append(batch, expired{txID: txID, listeners: entry.listeners})
		delete(n.handlers, txID)
	}
	n.handlersMu.Unlock()

	if len(batch) == 0 {
		return
	}

	txIDs := make([]string, 0, len(batch))
	for _, e := range batch {
		txIDs = append(txIDs, e.txID)
	}
	logger.Debugf("Sweeping %d expired finality listener(s)", len(txIDs))

	// Phase 2: one batched query, outside the lock. Best-effort.
	var statuses map[string]int32
	if n.queryService != nil {
		var err error
		statuses, err = n.queryService.GetTransactionStatuses(txIDs)
		if err != nil {
			logger.Warnf("Could not resolve status of %d expired listener(s), reporting Unknown: %v", len(txIDs), err)
			statuses = nil
		}
	}

	// Phase 3: notify, outside the lock.
	for _, e := range batch {
		outcome := txOutcome{status: fdriver.Unknown}
		if st, ok := statuses[e.txID]; ok {
			outcome = txOutcome{status: statusFromCommitter(committerpb.Status(st))}
		}
		for _, h := range e.listeners {
			n.invokeHandler(ctx, h, e.txID, outcome)
		}
	}
}
```

`statusFromCommitter` is **already defined in Task 3 Step 4** — do not redeclare it here. It is reused
so a queried status and a streamed status are interpreted identically.

`GetTransactionStatuses` returns `map[string]int32` of raw committer status values, hence the
`committerpb.Status(st)` conversion.

- [ ] **Step 7: Extract `invokeHandler` and reuse it in the dispatcher**

The goroutine-per-handler-with-timeout block currently lives inline in the dispatcher (`nlm.go:144-162`). Extract it so the sweeper reuses the same timeout protection:

```go
// invokeHandler runs one listener in its own goroutine, bounded by
// handlerTimeout. A listener that ignores context cancellation leaks its
// goroutine, which is preferable to blocking the dispatcher.
func (n *notificationListenerManager) invokeHandler(ctx context.Context, h fabric.FinalityListener, txID string, outcome txOutcome) {
	go func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, n.handlerTimeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			h.OnStatus(timeoutCtx, txID, outcome.status, outcome.message)
			close(done)
		}()

		select {
		case <-done:
			// Handler completed within timeout
		case <-timeoutCtx.Done():
			logger.Warnf("OnStatus handler timed out for txID=%s (timeout=%s)", txID, n.handlerTimeout)
		}
	}()
}
```

Then replace the dispatcher's inline loop (`nlm.go:144-162`) with:

```go
			for _, c := range calls {
				n.invokeHandler(gCtx, c.handler, c.txID, txOutcome{status: c.status, message: c.message})
			}
```

**Caution:** `handlerTimeout` is zero in `setupTest` (it is never set, `nlm_test.go:108-113`), so `context.WithTimeout(ctx, 0)` yields an already-expired context. That is pre-existing behaviour in the dispatcher path and the existing tests pass regardless because `OnStatus` still runs in its own goroutine and completes. Do **not** "fix" this here — changing it would alter existing test behaviour and is out of scope.

- [ ] **Step 8: Add the ticker to the dispatcher select**

In the dispatcher goroutine (`nlm.go:106`), before the `for` loop:

```go
		// Sweep from the dispatcher rather than a separate goroutine: the
		// dispatcher is the only writer that deletes entries on the notification
		// path, so a sweep can never interleave with a dispatch. That removes the
		// notification-vs-expiry race by construction rather than by locking.
		sweepEvery := n.sweepInterval
		if sweepEvery <= 0 {
			sweepEvery = DefaultSweepInterval
		}
		ticker := time.NewTicker(sweepEvery)
		defer ticker.Stop()
```

and add the third case to the `select`:

```go
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case resp = <-n.responseQueue:
			case <-ticker.C:
				n.sweepExpired(gCtx)
				continue
			}
```

The `continue` matters: a tick carries no response, so falling through to `parseResponse(resp)` would re-dispatch the previous response. Verify `resp` is declared outside the loop (it is, `nlm.go:113`) — this is exactly why the `continue` is required.

- [ ] **Step 9: Run tests to verify they pass**

```bash
go test -race -run TestSweepExpired ./platform/fabricx/core/finality/ -v
go test -race ./platform/fabricx/core/finality/ -count=1
```

Expected: all PASS, including `nlm_deadlock_poc_test.go`.

- [ ] **Step 10: Verify the sweeper cannot stall the dispatcher**

A slow query must not block notification delivery. Run the package with a stress count to shake out ordering flakes:

```bash
go test -race -count=5 ./platform/fabricx/core/finality/
```

Expected: PASS every iteration. If `Notification_Wins_No_Double_Invoke` panics with "negative WaitGroup counter", the sweeper and dispatcher are both settling the same entry — re-check that Phase 1 deletes under the same lock the dispatcher uses.

- [ ] **Step 11: Run gates**

```bash
make unit-tests
make checks
```

- [ ] **Step 12: Commit**

```bash
git add platform/fabricx/core/finality/
git commit -s -m "fix(fabricx): expire stale finality listeners locally

handlers only shrank when the committer notified, so a missed notification
retained the entry and the listener closure it pins forever. The 10s field in
NotificationRequest is a remote timeout delivered over the same stream, not a
local one.

Adds a per-entry deadline swept from the existing dispatcher goroutine, which
queries GetTransactionStatuses for expiring txIDs so expiry reports the true
status rather than a blind Unknown -- the committer's timeout is documented
non-strict, so guessing could report Unknown for a committed tx.

Entries are deleted before the query: cleanup must not depend on a call to the
same service whose silence caused the leak.

listenerTTL == 0 disables expiry, so a missing wire-up degrades to current
behaviour rather than expiring everything at once.

Refs #1626"
```

---

### Task 5: Wire the query service through DI

**Files:**
- Modify: `platform/fabricx/core/finality/provider.go:53-61`, `:63-73`, `:108`, `:138-156`
- Test: `platform/fabricx/core/finality/provider_test.go`

**Interfaces:**
- Consumes: `TxStatusQuerier`, `DefaultListenerTTL`, `DefaultSweepInterval` (Task 4).
- Produces: `NewListenerManagerProvider(GRPCClientProvider, ServiceConfigProvider, queryservice.Provider) *Provider` — a **breaking signature change** to an exported constructor.

- [ ] **Step 1: Check the existing provider test's construction sites**

```bash
grep -n "NewListenerManagerProvider\|newNotificationManager\|newNotifiWithGRPC" platform/fabricx/core/finality/provider_test.go
```

Every call site found must be updated in Step 4. Read the surrounding setup before editing so the third argument matches the test's existing mock style.

- [ ] **Step 2: Add the field and constructor parameter**

In `provider.go`, add to the `Provider` struct (after `grpcClientProvider`, `provider.go:68`):

```go
	queryServiceProvider queryservice.Provider
```

Change `NewListenerManagerProvider` (`provider.go:53-61`):

```go
func NewListenerManagerProvider(
	grpcClientProvider GRPCClientProvider,
	configProvider ServiceConfigProvider,
	queryServiceProvider queryservice.Provider,
) *Provider {
	return &Provider{
		grpcClientProvider:     grpcClientProvider,
		configProvider:         configProvider,
		queryServiceProvider:   queryServiceProvider,
		managers:               make(map[string]ListenerManager),
		newNotificationManager: newNotifiWithGRPC,
		// Note: baseCtx will be initialized in the Initialize method.
	}
}
```

Add the import:

```go
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
```

There is no import cycle — `queryservice` does not depend on `finality`.

- [ ] **Step 3: Thread `channel` and the query service into the factory**

The `newNotificationManager` function field (`provider.go:66`) currently takes `(network string, gcp GRPCClientProvider)`. `queryservice.Provider.Get` needs `(network, channel)`, and `NewManager` already has both (`provider.go:91`). Change the field type:

```go
	newNotificationManager func(network, channel string, gcp GRPCClientProvider, qsp queryservice.Provider) (*notificationListenerManager, error)
```

Update the call site in `NewManager` (`provider.go:108`):

```go
	lm, err := p.newNotificationManager(network, channel, p.grpcClientProvider, p.queryServiceProvider)
```

Rewrite `newNotifiWithGRPC` (`provider.go:138-156`):

```go
// newNotifiWithGRPC creates and initializes a notificationListenerManager using
// the GRPCClientProvider.
func newNotifiWithGRPC(network, channel string, grpcClientProvider GRPCClientProvider, qsp queryservice.Provider) (*notificationListenerManager, error) {
	cc, err := grpcClientProvider.NotificationServiceClient(network)
	if err != nil {
		return nil, errors.Wrapf(err, "get grpc client for notification service [network=%s]", network)
	}

	// Create the gRPC client stub for the Notifier service
	notifyClient := committerpb.NewNotifierClient(cc)

	nlm := &notificationListenerManager{
		notifyClient:   notifyClient,
		requestQueue:   make(chan *committerpb.NotificationRequest),  // Queue for outgoing requests to the committer
		responseQueue:  make(chan *committerpb.NotificationResponse), // Queue for incoming responses/notifications
		handlers:       make(map[driver.TxID]*handlerEntry),          // Map: txID -> listeners + local expiry deadline
		handlerTimeout: DefaultHandlerTimeout,
		listenerTTL:    DefaultListenerTTL,
		sweepInterval:  DefaultSweepInterval,
	}

	// The query service lets local expiry report the true transaction status
	// instead of a blind Unknown. It is optional: if it cannot be resolved the
	// sweeper still runs and still removes entries.
	if qsp != nil {
		qs, err := qsp.Get(network, channel)
		if err != nil {
			logger.Warnf("No query service for [network=%s channel=%s]; expired listeners will report Unknown: %v", network, channel, err)
		} else {
			nlm.queryService = qs
		}
	}

	return nlm, nil
}
```

Resolution failure is logged, not fatal: the leak fix must not become a new startup failure mode.

- [ ] **Step 4: Update `provider_test.go` call sites**

Update every site found in Step 1. For `newNotificationManager` stubs, the function literal gains two parameters:

```go
	p.newNotificationManager = func(network, channel string, gcp GRPCClientProvider, qsp queryservice.Provider) (*notificationListenerManager, error) {
		return nlm, nil
	}
```

For `NewListenerManagerProvider` calls, pass `nil` as the third argument where the test does not exercise the query service.

- [ ] **Step 5: Verify `dig` still resolves the container**

`sdk/dig/sdk.go` needs **no edit** — `queryservice.Provider` is already provided (`sdk.go:64`) and `dig` injects the new parameter by type. Confirm both modules still build:

```bash
go build ./...
go vet ./platform/fabricx/...
```

Expected: clean.

- [ ] **Step 6: Run the package and gates**

```bash
go test -race ./platform/fabricx/core/finality/ -count=1
make unit-tests
make checks
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add platform/fabricx/core/finality/
git commit -s -m "feat(fabricx): wire query service into the finality listener manager

Lets local expiry report a transaction's true status instead of Unknown.
Resolved per network/channel; a resolution failure is logged and the sweeper
still removes entries, so this cannot become a new startup failure mode.

sdk/dig/sdk.go needs no change: queryservice.Provider is already registered in
the same container, so dig injects the new constructor parameter by type.

Refs #1626"
```

---

### Task 6: Verify end to end and update the tidy/docs surface

**Files:**
- Verify only; no source changes expected.

**Interfaces:**
- Consumes: everything from Tasks 1-5.
- Produces: nothing.

- [ ] **Step 1: Full unit suite with race detection, repeated**

```bash
go test -race -count=3 ./platform/fabricx/...
```

Expected: PASS all three iterations. Repetition matters because the sweeper introduces the package's first timing-driven code path.

- [ ] **Step 2: Confirm the deadlock regression guard still holds**

```bash
go test -race -count=5 -run TestAddFinalityListenerRecoversAfterStreamFailure ./platform/fabricx/core/finality/ -v
```

Expected: PASS every run. This test guards a previously fixed deadlock; the ticker added a third case to the same `select`, so this is the highest-value regression check in the plan.

- [ ] **Step 3: Verify no leak remains under the original scenario**

Confirm by inspection that all four removal paths now exist. Expect exactly these `delete(n.handlers` / `clear(` sites:

```bash
grep -n "delete(n.handlers\|clear(n.handlers" platform/fabricx/core/finality/nlm.go
```

Expected: the dispatcher delete, the `AddFinalityListener` send-failure rollback, the `RemoveFinalityListener` delete, the `clear` on teardown, and the new `sweepExpired` delete.

- [ ] **Step 4: Module hygiene**

No dependencies were added, but confirm nothing drifted:

```bash
make tidy
git diff --stat
```

Expected: no changes to any `go.mod`/`go.sum`. If `make tidy` dirties them, investigate before committing — it means an import was added that the plan did not intend.

- [ ] **Step 5: Final gates**

```bash
make unit-tests
make checks
make lint
```

Expected: all PASS. `make lint` is included here (not per-task) because it is the slowest gate.

- [ ] **Step 6: Commit any tidy fallout, else skip**

```bash
git status --short
# Only if make tidy or make lint changed files:
git add -A && git commit -s -m "chore(fabricx): tidy after finality listener expiry work

Refs #1626"
```

---

## Post-Implementation

**Not part of this plan** — raise as separate issues (recorded in the spec's *Out of scope*):

1. **Hardcoded 10s request timeout** (`nlm.go:239`) overrides the committer's configured default; `notify.proto:35-36` says an unset value means "use the server default".
2. **Panicking listeners** take the process down via the unrecovered goroutine in `invokeHandler`. Pre-existing; the sweeper reuses that path so it inherits the exposure.
3. **`RemoteQueryService` ignores caller contexts** (`queryservice/query.go:78,115,136` use `context.Background()`), making queries uncancellable with a 30s ceiling. Fixing it would unblock a registration-time already-final check, which this design deliberately deferred.
4. **`committerService.AddFinalityListener`** (`core/channel/wrappers.go:237-244`) returns `nil` when its type assertion fails, swallowing the new empty-txID error through that path.

**PR description should note:** the spec commit is already on this branch; `parseResponse` changed signature (package-private, single caller); `NewListenerManagerProvider` changed signature (exported — check for downstream callers outside this repo before release).

## Self-Review Notes

Checked against the spec:

- Layers 1-5 → Tasks 1, 3, 4, 2, 5 respectively. All covered.
- Every spec test case has a task step: empty-txID (T1), rejection + reason (T3), precedence (T3), sweeper-Valid / absent-Unknown / query-error / nil-QS / no-double-invoke (T4), deadlock regression (T4 S10, T6 S2).
- **Corrected from the spec:** the spec estimated "~18 mechanical test sites". The real number is **12** — the ~20 `_, exists :=` sites compile unchanged because they discard the map value. Task 2 lists the 12 exactly.
- **Found while planning:** `mockListener.OnStatus` discards `errMsg` (`nlm_test.go:46-52`), so the rejection-reason assertion is impossible without extending the mock. Added as T3 S1 — the spec did not anticipate it.
- **Found while planning:** the ticker case needs `continue`, or a tick falls through and re-dispatches the stale `resp` (declared outside the loop, `nlm.go:113`). Called out in T4 S8.
- **Found while planning:** `handlerTimeout` is zero under `setupTest`, so the extracted `invokeHandler` inherits an already-expired context in tests. Pre-existing; explicitly flagged do-not-fix in T4 S7.
- **Fixed during review:** `statusFromCommitter` was defined twice (once in T3, once in T4) — a
  compile error. It is now defined once in T3 S4, extracted from the existing `parseResponse` switch,
  and reused by T4's sweeper so a queried status and a streamed status are interpreted identically.
- **Verified against source, not assumed:**
  - `GetRejectedTxIds()` / `GetTxIds()` / `GetReason()` exist on the generated types
    (`notify.pb.go:144,190,197`), so T3's code compiles.
  - `fdriver.ValidationCode` is `= int` (an alias, `platform/fabric/driver/committer.go:18`), so
    `txOutcome.status int` is correct. Note the unrelated `common/driver.TxStatusCode int32`
    (`platform/common/driver/vault.go:44`) is a *different* type — do not mix them.
- Type consistency: `txOutcome{status, message}` defined T3 S4, used T3 S5 and T4 S6/S7. `handlerEntry{listeners, expiresAt}` defined T2 S1, used T2/T4. `TxStatusQuerier` defined T4 S4, consumed T4 S1 (stub) and T5 S3. `statusFromCommitter` defined T3 S4, used T3 S4 and T4 S6.
