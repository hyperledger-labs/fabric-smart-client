/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

var logger = logging.MustGetLogger()

// DefaultHandlerTimeout is the maximum time allowed for a finality listener
// handler to complete. If a handler exceeds this timeout, a warning is logged
// and the handler is abandoned. Note: Handlers that ignore context cancellation
// will leak goroutines, but this is preferable to blocking the dispatcher.
const DefaultHandlerTimeout = 5 * time.Second

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

// handlerEntry holds the listeners registered for one transaction, plus the
// deadline after which the entry is swept locally. expiresAt is zero when
// local expiry is disabled (listenerTTL == 0).
type handlerEntry struct {
	listeners []fabric.FinalityListener
	expiresAt time.Time
}

type notificationListenerManager struct {
	notifyClient   committerpb.NotifierClient
	requestQueue   chan *committerpb.NotificationRequest
	responseQueue  chan *committerpb.NotificationResponse
	handlerTimeout time.Duration

	// queryService resolves the true status of expiring entries. When nil, the
	// sweeper reports Unknown instead of querying.
	queryService TxStatusQuerier
	// listenerTTL bounds how long an entry may stay unresolved. Zero disables
	// local expiry entirely, which is what the test setup relies on and what a
	// missing wire-up degrades to.
	listenerTTL time.Duration
	// sweepInterval is the sweep tick period. Ignored when listenerTTL is zero.
	sweepInterval time.Duration

	handlers   map[driver.TxID]*handlerEntry
	handlersMu sync.RWMutex

	// streamCtx holds the errgroup context of the currently active listen()
	// call, if any. Its Done() channel closes as soon as that stream fails
	// (errgroup cancels it on the first goroutine error, without waiting for
	// the others to unwind) or the parent context is canceled. AddFinalityListener
	// uses it to fail fast on a dead stream instead of blocking forever on
	// requestQueue.
	streamCtx atomic.Pointer[context.Context]
}

// Listen is a blocking method that runs the notification listener stream.
func (n *notificationListenerManager) listen(ctx context.Context) error {
	logger.Debugf("Notification listener stream starting.")
	notifyStream, err := n.notifyClient.OpenNotificationStream(ctx)
	if err != nil {
		return err
	}
	// Use the base context for errgroup
	g, gCtx := errgroup.WithContext(ctx)

	// Publish gCtx so AddFinalityListener can select on it instead of
	// blocking forever on requestQueue once this stream dies. Deliberately
	// left in place (not cleared) after listen() returns: gCtx.Done() stays
	// closed forever once this stream fails, which is exactly the permanent
	// "this stream is dead" signal AddFinalityListener needs until a new
	// listen() call replaces it with a fresh, live gCtx.
	n.streamCtx.Store(&gCtx)

	// spawn stream receiver
	g.Go(func() error {
		for {
			res, err := notifyStream.Recv()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case n.responseQueue <- res:
			}
		}
	})

	// spawn stream sender
	g.Go(func() error {
		var req *committerpb.NotificationRequest
		for {
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case req = <-n.requestQueue:
			}

			if err := notifyStream.Send(req); err != nil {
				return err
			}
		}
	})

	// spawn notification dispatcher
	g.Go(func() error {
		type handlerCall struct {
			handler fabric.FinalityListener
			txID    string
			status  int
			message string
		}

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

		var resp *committerpb.NotificationResponse
		for {
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case resp = <-n.responseQueue:
			case <-ticker.C:
				n.sweepExpired(gCtx)
				continue
			}

			res := parseResponse(resp)

			// Collect handlers under lock, then release before spawning goroutines.
			// This minimizes lock hold time — only map lookups and deletes happen
			// under the lock. Goroutine scheduling happens entirely outside.
			var calls []handlerCall

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

			// Invoke each handler in its own goroutine with a timeout.
			// If a handler ignores the context and never returns, the goroutine
			// will leak — but the dispatcher remains unblocked.
			for _, c := range calls {
				n.invokeHandler(gCtx, c.handler, c.txID, txOutcome{status: c.status, message: c.message})
			}
		}
	})

	err = g.Wait()
	logger.Debugf("Notification listener stream stopped.")

	// Cleanup handlers map when listen() exits
	n.handlersMu.Lock()
	clear(n.handlers)
	n.handlersMu.Unlock()
	logger.Debugf("Cleared handlers map on listen() exit")

	return err
}

// sweepExpired settles entries whose local deadline has passed.
//
// Entries are deleted from the map BEFORE the status query, deliberately:
// cleanup must never depend on a network call. The query service and the
// notification stream both talk to the same committer, so the fault that lost
// the notification is correlated with the query failing -- if removal waited on
// a successful query, the sweeper would be useless in exactly the situation it
// exists for. A failed query therefore degrades to Unknown, not to a retained
// entry.
//
// Phase 1 (collect + delete) runs INLINE in the dispatcher goroutine: the
// dispatcher being the only goroutine that deletes on the notification path is
// what makes a double-invoke impossible by construction, and moving the delete
// off it would give that up. Phases 2-3 (the status query and the handler
// notifications) run in a separate goroutine because GetTransactionStatuses is
// a synchronous, caller-context-ignoring network call: on the dispatcher it
// backpressures through the unbuffered response queue into Recv and freezes
// notification delivery for the whole channel for up to the query service's own
// RequestTimeout -- long enough for an in-flight COMMITTED notification's own
// TTL to elapse and be mis-settled as Unknown by the next sweep. Do not
// "simplify" this back inline.
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

	// Phases 2 and 3 off the dispatcher: batch is already a private copy and its
	// entries are already out of the map, so nothing here needs handlersMu and no
	// ordering guarantee depends on running them inline.
	go func() {
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
	}()
}

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

// expiryFor returns the local expiry deadline for an entry created at now, or
// the zero time when local expiry is disabled.
func (n *notificationListenerManager) expiryFor(now time.Time) time.Time {
	if n.listenerTTL <= 0 {
		return time.Time{}
	}
	return now.Add(n.listenerTTL)
}

// AddFinalityListener registers a listener to be notified when the transaction with the given txID reaches finality.
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

	n.handlersMu.Lock()
	defer n.handlersMu.Unlock()

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

	n.handlers[txID] = &handlerEntry{
		listeners: []fabric.FinalityListener{listener},
		expiresAt: n.expiryFor(time.Now()),
	}

	// this is our first listener registered for the given txID
	txIDs := []string{txID}
	req := &committerpb.NotificationRequest{
		TxStatusRequest: &committerpb.TxIDsBatch{
			TxIds: txIDs,
		},
		// TODO: set a proper timeout
		Timeout: durationpb.New(10 * time.Second),
	}

	// Guard the send against a dead stream: once listen()'s errgroup context
	// is done (the stream failed, or listen()'s parent context was
	// canceled), nothing will ever drain requestQueue again, so sending
	// unconditionally would block forever -- see the streamCtx field doc.
	// We still hold handlersMu here (never released since the top of this
	// call), so no other goroutine can have joined this txID's handler list
	// in the meantime; it is safe to simply undo our own registration on
	// failure rather than to hand the send off to a would-be next caller.
	var done <-chan struct{}
	if sc := n.streamCtx.Load(); sc != nil {
		done = (*sc).Done()
	}

	select {
	case n.requestQueue <- req:
		return nil
	case <-done:
		delete(n.handlers, txID)
		return errors.Errorf("notification stream unavailable, cannot register listener for txID=%s", txID)
	}
}

// RemoveFinalityListener unregisters a previously registered listener for the given txID.
func (n *notificationListenerManager) RemoveFinalityListener(txID string, listener fabric.FinalityListener) error {
	if listener == nil {
		return errors.New("listener nil")
	}

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
}
