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
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
)

var logger = logging.MustGetLogger()

// The notification service's configurable defaults live in committer/config, the
// single source of truth; this package consumes them via config.Config rather than
// defining its own copies.

// handlerCall is one OnStatus invocation: the unit of work queued for the handler
// pool.
type handlerCall struct {
	handler fabric.FinalityListener
	txID    string
	status  int
}

// handlerEntry holds the listeners waiting on one transaction, together with the
// deadline after which they are settled locally. A zero expiresAt means the entry
// never expires, which is what a manager built with listenerTTL == 0 produces.
type handlerEntry struct {
	listeners []fabric.FinalityListener
	expiresAt time.Time
}

type notificationListenerManager struct {
	notifyClient   committerpb.NotifierClient
	requestQueue   chan *committerpb.NotificationRequest
	responseQueue  chan *committerpb.NotificationResponse
	handlerTimeout time.Duration

	// handlerWorkers is how many OnStatus callbacks may run at once, and so also how
	// many stuck listeners it takes to stop delivering notifications on this stream.
	handlerWorkers int

	// callQueue buffers callbacks for the handler pool. It absorbs bursts: one
	// notification response can carry far more transactions than there are workers,
	// and without a buffer everything past that would be dropped even though the
	// listeners are healthy and about to become free again.
	callQueue chan handlerCall

	// requestTimeout is sent to the committer as the outbound NotificationRequest's
	// Timeout, so it gives up and replies once it passes rather than us aborting the
	// gRPC call locally and marking transactions the committer may already have an
	// answer for as Unknown. See notify.proto's Timeout field doc.
	requestTimeout time.Duration

	// listenerTTL is how long an entry may stay unresolved before the sweeper
	// settles it with Unknown. Zero disables local expiry entirely.
	listenerTTL time.Duration
	// sweepInterval is the sweep tick period. Ignored when listenerTTL is zero;
	// falls back to config.DefaultSweepInterval if unset.
	sweepInterval time.Duration

	handlers   map[driver.TxID]*handlerEntry
	handlersMu sync.RWMutex

	// streamCtx holds the context of the currently active listen() call, if any.
	// Its Done() channel closes as soon as that stream fails (the first stream
	// goroutine to error cancels it, without waiting for the others to unwind) or
	// the parent context is canceled. AddFinalityListener
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
	// One group for the three stream goroutines: receiver, sender, dispatcher.
	g, gCtx := errgroup.WithContext(ctx)

	// Publish gCtx so AddFinalityListener can select on it instead of
	// blocking forever on requestQueue once this stream dies. Deliberately
	// left in place (not cleared) after listen() returns: gCtx.Done() stays
	// closed forever once this stream fails, which is exactly the permanent
	// "this stream is dead" signal AddFinalityListener needs until a new
	// listen() call replaces it with a fresh, live gCtx.
	n.streamCtx.Store(&gCtx)

	// The workers are deliberately not part of g: a listener that ignores cancellation
	// never returns, so neither does its worker, and g.Wait() would then never return
	// -- hanging listen() and node shutdown. waitHandlers waits on the WaitGroup with
	// a timeout instead. It is a local so a second listen() call cannot Add to a group
	// an earlier one is still waiting on.
	//
	// poolCtx strips cancellation and relies on stopHandlers: an inherited cancel
	// would hand in-flight callbacks a dead context, and callHandler's timeout would
	// expire instantly.
	poolCtx, stopHandlers := context.WithCancel(context.WithoutCancel(ctx))
	defer stopHandlers()

	var handlerPool sync.WaitGroup
	for range n.handlerWorkers {
		handlerPool.Add(1)
		go func() {
			defer handlerPool.Done()
			for {
				select {
				case c := <-n.callQueue:
					n.callHandler(poolCtx, c)
				case <-poolCtx.Done():
					// Drain before exiting. dispatch already deleted these callbacks'
					// handlers entries, so settleAllAndClear will not settle them:
					// returning here loses the notification outright. Bounded by the
					// queue's length at this instant.
					drainCtx := context.WithoutCancel(poolCtx) // poolCtx is already done
					for {
						select {
						case c := <-n.callQueue:
							n.callHandler(drainCtx, c)
						default:
							return
						}
					}
				}
			}
		}()
	}

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
		// Sweep from this goroutine rather than a dedicated one: it is already the
		// only goroutine that deletes entries on the notification path, so a sweep
		// can never interleave with a dispatch and no listener can be settled twice.
		sweepEvery := n.sweepInterval
		if sweepEvery <= 0 {
			sweepEvery = config.DefaultSweepInterval
		}
		ticker := time.NewTicker(sweepEvery)
		defer ticker.Stop()

		for {
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case <-ticker.C:
				n.sweepExpired()
			case resp := <-n.responseQueue:
				n.dispatch(resp)
			}
		}
	})

	err = g.Wait()
	logger.Debugf("Notification listener stream stopped.")

	// Stop the workers and give them a bounded window to finish.
	stopHandlers()
	n.waitHandlers(&handlerPool)

	// The stream is gone, so nothing will ever notify these listeners. Settle them
	// with Unknown instead of dropping them silently, so anyone blocked in IsFinal
	// is released now rather than waiting out their own context.
	//
	// ctx, not gCtx: the stream context is already cancelled by the time g.Wait()
	// returns, and invokeHandler derives its handler timeout from what we pass, so
	// gCtx would hand every listener a dead context and deliver nothing. Strip
	// cancellation from the parent too -- listen() is often returning *because*
	// ctx was cancelled, and these callbacks still need to run.
	n.settleAllAndClear(context.WithoutCancel(ctx), fdriver.Unknown)

	return err
}

// settleAllAndClear empties the handlers map, invoking every listener still in it
// with the given status. Used on stream teardown, where no notification can
// arrive any more.
//
// Does not go through the handler pool: the workers are already stopped by now, so
// queued callbacks would never be drained. Listeners are invoked directly instead,
// each via callHandlerBounded so one that ignores its context cannot block listen()
// from returning. Bounded by the listeners still unresolved at teardown.
func (n *notificationListenerManager) settleAllAndClear(ctx context.Context, status int) {
	type pending struct {
		txID      string
		listeners []fabric.FinalityListener
	}

	var batch []pending

	n.handlersMu.Lock()
	for txID, entry := range n.handlers {
		batch = append(batch, pending{txID: txID, listeners: entry.listeners})
	}
	clear(n.handlers)
	n.handlersMu.Unlock()

	if len(batch) == 0 {
		logger.Debugf("Cleared handlers map on listen() exit")
		return
	}

	logger.Debugf("Settling %d pending finality listener(s) with status %d on stream teardown", len(batch), status)

	for _, p := range batch {
		for _, h := range p.listeners {
			n.callHandlerBounded(ctx, handlerCall{handler: h, txID: p.txID, status: status})
		}
	}
}

// callHandlerBounded invokes one listener and gives up waiting for it after
// handlerTimeout, so a listener that ignores cancellation cannot block the caller
// indefinitely. Used only on the teardown path, where the handler pool is already
// stopped; the live paths get their bound from the pool's fixed size instead.
func (n *notificationListenerManager) callHandlerBounded(ctx context.Context, c handlerCall) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		n.callHandler(ctx, c)
	}()

	select {
	case <-done:
	case <-time.After(n.handlerTimeout):
		logger.Warnf(
			"OnStatus handler for txID=%s did not return within %s on stream teardown; abandoning the wait",
			c.txID, n.handlerTimeout)
	}
}

func parseResponse(resp *committerpb.NotificationResponse) map[string]int {
	res := make(map[string]int)

	// first parse all timeouts
	for _, txID := range resp.GetTimeoutTxIds() {
		res[txID] = fdriver.Unknown
	}

	var s int
	// next we parse the status events
	for _, r := range resp.GetTxStatusEvents() {

		txID := r.GetRef().GetTxId()
		status := r.GetStatus()

		logger.Debugf("transaction [%s] status [%s]", txID, status)

		switch status {
		case committerpb.Status_COMMITTED:
			s = fdriver.Valid
		case committerpb.Status_STATUS_UNSPECIFIED:
			s = fdriver.Unknown
		default:
			s = fdriver.Invalid
		}

		res[txID] = s
	}

	return res
}

// AddFinalityListener registers a listener to be notified when the transaction with the given txID reaches finality.
func (n *notificationListenerManager) AddFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error {
	if listener == nil {
		return errors.New("listener nil")
	}
	// An empty txID can never appear in a committer notification, so the entry it
	// would create is unremovable. The generic driver already rejects this (see
	// platform/common/core/generic/committer/listenermgr.go); keep the message
	// identical so both drivers behave the same for the same API call.
	if len(txID) == 0 {
		return errors.New("tx id must be not empty")
	}

	n.handlersMu.Lock()
	defer n.handlersMu.Unlock()

	if entry, existed := n.handlers[txID]; existed {
		if slices.Contains(entry.listeners, listener) {
			logger.Warnf("The exact same listener is already registered for txID=%v. Skipping.", txID)
			// Do not register the same instance twice
			return nil
		}
		// A joining listener inherits the existing deadline rather than extending
		// it, so a busy txID cannot keep its entry alive indefinitely.
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
		// Timeout tells the committer to reply once it passes rather than leaving
		// it to its own internal max-timeout. Without this, a client-side abort
		// (e.g. our own listenerTTL firing) can mark transactions Unknown that the
		// committer already knows the outcome of, because the committer never got
		// a reason to answer early. See notify.proto's Timeout field doc.
		Timeout: durationpb.New(n.requestTimeout),
	}

	// Guard the send against a dead stream: once listen()'s stream context
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
		// Mutating the entry in place keeps the existing deadline: removing one
		// listener must not extend the lifetime of the ones still waiting.
		entry.listeners = newHandlers
		logger.Debugf("Removed listener for txID=%s. %d listeners remaining.", txID, len(newHandlers))
	}

	return nil
}

// enqueueHandler buffers one OnStatus invocation for the handler pool. It never
// blocks: the caller is the dispatcher goroutine, which also drains the notification
// stream and runs the expiry sweeper, so blocking here would stall both.
//
// A full queue means callbacks are being produced faster than the pool retires them
// for as long as the buffer took to fill, i.e. listeners are slow or stuck. Only then
// is the invocation dropped, and a drop is not recovered: callers have already deleted
// the handlers entry, so the listener falls back to its own context deadline or to the
// listenerTTL sweeper's Unknown.
//
// Returns whether the callback was queued, so callers can aggregate drops into one
// warning per batch rather than one line per txID; see logDrops.
func (n *notificationListenerManager) enqueueHandler(c handlerCall) bool {
	if n.callQueue == nil {
		// No queue, so nothing will ever run this. Reachable only from a direct
		// dispatch/sweep call on a manager built outside newNotifiWithGRPC.
		logger.Warnf("no handler queue, dropping OnStatus for txID=%s", c.txID)
		return false
	}

	select {
	case n.callQueue <- c:
		return true
	default:
		// Logged at debug, not warn: a full queue drops every remaining call in the
		// batch, so warning here would emit one near-identical line per txID at the
		// notification rate. The caller aggregates into a single warning -- see
		// logDrops.
		logger.Debugf("handler queue full, dropped OnStatus for txID=%s", c.txID)
		return false
	}
}

// logDrops emits one aggregated warning for a batch in which some callbacks could
// not be queued. Aggregated on purpose: a full queue drops every remaining call, so
// per-txID warnings would flood the log at the notification rate exactly when an
// operator most needs to read it. Individual txIDs are at debug level.
func (n *notificationListenerManager) logDrops(dropped, total int) {
	if dropped == 0 {
		return
	}
	logger.Warnf(
		"dropped %d of %d finality callbacks: queue full with %d handler slots. "+
			"Either listeners are not keeping up (raise handlerWorkers), one is ignoring "+
			"its context, or this batch was larger than the limit. Affected listeners will "+
			"be settled with Unknown after listenerTTL.",
		dropped, total, n.handlerWorkers)
}

// waitHandlers waits for in-flight callbacks to finish, but only up to
// handlerTimeout. Bounded on purpose: a listener that ignores cancellation never
// returns, so an unconditional wait would let one misbehaving listener block
// shutdown forever. The timeout still reaps the common case tidily.
func (n *notificationListenerManager) waitHandlers(pool *sync.WaitGroup) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		pool.Wait()
	}()

	select {
	case <-done:
		logger.Debugf("all finality handler callbacks finished")
	case <-time.After(n.handlerTimeout):
		logger.Warnf(
			"finality handler callbacks still running after %s on stream teardown; "+
				"abandoning the wait (a listener is ignoring its context)",
			n.handlerTimeout)
	}
}

// callHandler invokes one listener synchronously, with a context bounded by
// handlerTimeout. The timeout is advisory: it cancels the listener's context but
// cannot force a return, so a listener that ignores it occupies this worker for as
// long as it runs. See config.DefaultHandlerWorkers.
func (n *notificationListenerManager) callHandler(ctx context.Context, c handlerCall) {
	timeoutCtx, cancel := context.WithTimeout(ctx, n.handlerTimeout)
	defer cancel()

	start := time.Now()
	c.handler.OnStatus(timeoutCtx, c.txID, c.status, "")

	// Warn only when this callback's own deadline passed while it was still running,
	// i.e. it ignored cancellation. Checking DeadlineExceeded specifically, not
	// timeoutCtx.Err() != nil: the parent is cancelled on stream teardown, which
	// makes Err() non-nil for every callback in flight at that moment and would
	// report healthy listeners as misbehaving.
	if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
		logger.Warnf(
			"OnStatus handler for txID=%s did not return before its deadline (took %s, timeout=%s), "+
				"blocking one of %d handler workers for that long; OnStatus must observe ctx.Done() and return promptly",
			c.txID, time.Since(start), n.handlerTimeout, n.handlerWorkers)
	}
}

// expiryFor returns the local deadline for an entry created at now, or the zero
// time when local expiry is disabled.
func (n *notificationListenerManager) expiryFor(now time.Time) time.Time {
	if n.listenerTTL <= 0 {
		return time.Time{}
	}
	return now.Add(n.listenerTTL)
}

// dispatch settles the listeners named by one notification response.
//
// Collects under the lock and notifies outside it, so only map lookups and
// deletes happen while handlersMu is held. Runs on the dispatcher goroutine,
// which is what lets sweepExpired share the same map without either path being
// able to settle a listener the other already settled.
// Takes no context: it only mutates the map and enqueues, and the enqueued work
// runs under the pool's own context rather than the dispatcher's.
func (n *notificationListenerManager) dispatch(resp *committerpb.NotificationResponse) {
	var calls []handlerCall

	n.handlersMu.Lock()
	for txID, status := range parseResponse(resp) {
		entry, ok := n.handlers[txID]
		if !ok {
			continue
		}
		delete(n.handlers, txID)
		for _, h := range entry.listeners {
			calls = append(calls, handlerCall{handler: h, txID: txID, status: status})
		}
	}
	n.handlersMu.Unlock()

	dropped := 0
	for _, c := range calls {
		if !n.enqueueHandler(c) {
			dropped++
		}
	}
	n.logDrops(dropped, len(calls))
}

// sweepExpired settles listeners whose local deadline has passed.
//
// Without this, the only steady-state path that removes a handlers entry is an
// inbound notification, so a committer that never reports on a transaction --
// because it dropped the subscription, is overloaded, or has a bug -- leaves the
// entry, and the listener closure it pins, in the map forever. The timeout we set
// on the outbound request does not help: it asks the *committer* to give up and
// reply, so it too depends on the stream we are no longer hearing from.
//
// Listeners are settled with Unknown, which is the same outcome the committer's
// own TimeoutTxIds path produces, so callers see nothing new. Note this can
// report Unknown for a transaction that did in fact commit, because the remote
// timeout is documented non-strict and a notification may arrive after we have
// given up; listenerTTL is configured well above requestTimeout to make that
// unlikely. Callers needing certainty can query the transaction status directly.
//
// Runs on the dispatcher goroutine (see the ticker in listen): the dispatcher is
// the only other goroutine that deletes entries on the notification path, so a
// sweep and a dispatch can never interleave and one listener can never be settled
// twice. Keep it that way -- moving this off the dispatcher would make
// double-invoke merely preventable rather than impossible.
// Takes no context, for the same reason as dispatch.
func (n *notificationListenerManager) sweepExpired() {
	if n.listenerTTL <= 0 {
		return
	}

	now := time.Now()

	type expired struct {
		txID      string
		listeners []fabric.FinalityListener
	}
	var batch []expired

	// Collect and delete under the lock; notify outside it, mirroring how the
	// dispatcher handles its own callbacks.
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

	logger.Debugf("Settling %d expired finality listener(s) with Unknown", len(batch))

	dropped, total := 0, 0
	for _, e := range batch {
		for _, h := range e.listeners {
			total++
			if !n.enqueueHandler(handlerCall{handler: h, txID: e.txID, status: fdriver.Unknown}) {
				dropped++
			}
		}
	}
	n.logDrops(dropped, total)
}
