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

// DefaultHandlerTimeout, DefaultHandlerWorkers, DefaultListenerTTL and
// DefaultSweepInterval live in committer/config, the single source of truth for
// the notification service's configurable defaults; this package consumes them via
// config.Config rather than defining its own copies.

// slotPollInterval is how often the queue feeder re-checks for a free handler slot
// while every slot is busy. Only reached when the pool is saturated, so it trades a
// little latency in an already-degraded state for keeping cancellation observable;
// see the feeder in listen().
const slotPollInterval = 2 * time.Millisecond

// handlerCall is one OnStatus invocation: the unit of work handed to the handler
// errgroup.
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

	// handlerWorkers is the limit set on the handler errgroup (see handlerGroup),
	// so it is how many OnStatus callbacks may run at once -- and therefore also
	// how many stuck listeners it takes to stop delivering notifications on this
	// stream entirely.
	handlerWorkers int

	// callQueue buffers callbacks between the dispatcher and the handler pool.
	//
	// The queue and the limit do different jobs, and both are needed. The limit
	// bounds how many listeners run at once, which is what stops a misbehaving
	// listener from growing goroutines without end. The queue absorbs bursts: one
	// notification response can carry far more transactions than there are slots,
	// and without a buffer everything past the limit would be dropped even though
	// the listeners are perfectly healthy and about to free their slots.
	//
	// Sends are non-blocking (see enqueueHandler): the only goroutine that enqueues
	// is also the one draining the notification stream and running the expiry
	// sweeper, so blocking here would stall both.
	callQueue chan handlerCall

	// handlerGroup runs listener callbacks, capped at handlerWorkers via SetLimit.
	// Deliberately a SEPARATE errgroup from the one listen() uses for its three
	// stream goroutines, for two reasons:
	//
	//   - Wait(): a listener that ignores cancellation and never returns keeps its
	//     goroutine alive forever. Were it in the stream group, that group's Wait()
	//     could never return, so listen() would never return and node shutdown
	//     would hang on one misbehaving callback.
	//   - Errors and limits: errgroup cancels its context on the first error and
	//     shares one limit across the whole group. Sharing would let handlers
	//     starve the stream goroutines of slots, and let a handler's error tear
	//     down the stream.
	//
	// Set up by listen() and nil until then; enqueueHandler tolerates that.
	// handlerCtx is the context callbacks receive, guarded by the same mutex.
	handlerGroup   *errgroup.Group
	handlerCtx     context.Context
	handlerGroupMu sync.RWMutex

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

	// Set up the handler errgroup: SetLimit caps concurrent OnStatus callbacks. See
	// the handlerGroup field for why this is a separate group from g, and callQueue
	// for why a queue sits in front of the limit.
	//
	// hCtx strips cancellation from ctx and relies on stopHandlers instead: a
	// callback that is mid-flight when ctx is canceled would otherwise be handed an
	// already-dead context, and callHandler's timeout would expire instantly,
	// delivering nothing.
	hCtx, stopHandlers := context.WithCancel(context.WithoutCancel(ctx))
	defer stopHandlers()

	hg, hgCtx := errgroup.WithContext(hCtx)
	hg.SetLimit(n.handlerWorkers)

	n.handlerGroupMu.Lock()
	n.handlerGroup = hg
	n.handlerCtx = hgCtx
	n.handlerGroupMu.Unlock()

	// Spawn the queue feeder: it moves buffered callbacks into the handler group,
	// waiting for a free slot rather than dropping. Waiting is correct *here* --
	// this is a dedicated goroutine, not the dispatcher, so it costs nothing but the
	// feeder's own progress. It is what lets a burst larger than handlerWorkers
	// still be delivered in full.
	g.Go(func() error {
		for {
			var c handlerCall
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case c = <-n.callQueue:
			}

			for !hg.TryGo(func() error {
				n.callHandler(hgCtx, c)
				return nil
			}) {
				// All slots busy. Wait for one to free, but stay cancellable.
				select {
				case <-gCtx.Done():
					return gCtx.Err()
				case <-time.After(slotPollInterval):
				}
			}
		}
	})

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

	// Cancel the handler context so in-flight callbacks that do observe
	// cancellation wind down, then give them a bounded window to finish.
	//
	// hg.Wait() is deliberately NOT waited on unconditionally: it returns only once
	// every callback has returned, and a listener that ignores cancellation never
	// does. Waiting with a timeout keeps the common case tidy (callbacks finish,
	// their goroutines are reaped before listen() returns) without letting one
	// misbehaving listener block shutdown indefinitely.
	stopHandlers()
	n.waitHandlers()

	// The stream is gone, so nothing will ever notify these listeners. Settle them
	// with Unknown instead of dropping them silently, so anyone blocked in IsFinal
	// is released now rather than waiting out their own context.
	//
	// ctx, not gCtx: the errgroup context is already cancelled by the time g.Wait()
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
// Does not go through the handler errgroup: this runs after listen() has already
// cancelled the handler context and waited out the in-flight callbacks, so its
// slots may still be held by listeners that ignored cancellation. Routing these
// final callbacks through TryGo would see those held slots and drop the very
// notifications this function exists to deliver.
//
// Each listener is therefore invoked directly, in its own goroutine, and waited
// for only up to handlerTimeout. Waiting inline instead would let a single
// listener that ignores its context block listen() from ever returning, hanging
// node shutdown -- the same failure the separate handler group avoids (see
// listen). The goroutines here are bounded by the number of listeners still
// unresolved at teardown, which happens once per stream death, so this is not a
// path that can grow without limit.
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
		// Mutating the entry in place keeps the existing deadline: removing one
		// listener must not extend the lifetime of the ones still waiting.
		entry.listeners = newHandlers
		logger.Debugf("Removed listener for txID=%s. %d listeners remaining.", txID, len(newHandlers))
	}

	return nil
}

// enqueueHandler buffers one OnStatus invocation for the handler pool. It never
// blocks: the caller is the dispatcher goroutine, which also drains the
// notification stream and runs the expiry sweeper, so blocking here would stall
// notification delivery for every other transaction as well.
//
// The queue is what makes a burst survivable. One notification response can carry
// many more transactions than there are handler slots, and they are dispatched in a
// tight loop; buffering them lets the feeder hand them to the pool as slots free,
// so a batch far larger than handlerWorkers is still delivered in full as long as
// the listeners themselves return.
//
// A full queue therefore means something worse than a burst: callbacks are being
// produced faster than the pool can retire them for as long as the buffer took to
// fill, which in practice means listeners are slow or stuck. Only then is the
// invocation dropped.
//
// A dropped invocation is not recovered: both callers have already deleted the
// handlers entry under the lock by this point, so the listener never receives a
// callback and whoever is waiting on it (e.g. IsFinal) falls back to its own
// context deadline -- or to the listenerTTL sweeper's Unknown, whichever comes
// first.
//
// Returns whether the callback was queued, so callers can aggregate drops into a
// single warning per batch rather than one line per txID; see logDrops.
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
// handlerTimeout. See the call site in listen() for why the wait is bounded.
func (n *notificationListenerManager) waitHandlers() {
	n.handlerGroupMu.RLock()
	hg := n.handlerGroup
	n.handlerGroupMu.RUnlock()

	if hg == nil {
		return
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = hg.Wait() // the callbacks never return an error; see enqueueHandler
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
// handlerTimeout.
//
// The timeout is advisory: it cancels the context handed to the listener, but
// nothing forces a listener that ignores cancellation to return. Such a listener
// occupies this worker for as long as it runs, and once every worker is occupied
// no notification can be delivered on this stream. That is deliberate -- it
// bounds a misbehaving listener's cost to throughput rather than letting it grow
// goroutines without limit -- but it is why OnStatus implementations MUST observe
// ctx.Done() and return promptly.
func (n *notificationListenerManager) callHandler(ctx context.Context, c handlerCall) {
	timeoutCtx, cancel := context.WithTimeout(ctx, n.handlerTimeout)
	defer cancel()

	start := time.Now()
	c.handler.OnStatus(timeoutCtx, c.txID, c.status, "")

	// Warn only when the handler was still running after its deadline passed, i.e.
	// it ignored cancellation for a while. Checking the context rather than the
	// elapsed time keeps this quiet for handlers that return promptly, and correct
	// when handlerTimeout is zero.
	if timeoutCtx.Err() != nil {
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
