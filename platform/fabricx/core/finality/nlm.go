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

	// status is the committer's answer, remembered because the queue was full when it
	// arrived. Its presence makes the entry due on the next sweep whatever expiresAt
	// says. A pointer because Unknown is itself a valid status: nil means "never heard".
	status *int
}

type notificationListenerManager struct {
	notifyClient   committerpb.NotifierClient
	requestQueue   chan *committerpb.NotificationRequest
	responseQueue  chan *committerpb.NotificationResponse
	handlerTimeout time.Duration

	// handlerWorkers is how many OnStatus callbacks may run at once, and so also how
	// many stuck listeners it takes to stop delivering notifications on this stream.
	handlerWorkers int

	// handlerQueueSize is the capacity of the callback queue listen() creates. It absorbs
	// bursts: one response can name far more transactions than there are workers.
	handlerQueueSize int

	// requestTimeout is sent to the committer as the outbound NotificationRequest's
	// Timeout, so it gives up and replies once it passes rather than us aborting the
	// gRPC call locally and marking transactions the committer may already have an
	// answer for as Unknown. See notify.proto's Timeout field doc.
	requestTimeout time.Duration

	// listenerTTL is how long an entry may stay unresolved before the sweeper
	// settles it with Unknown. Zero disables local expiry entirely.
	listenerTTL time.Duration
	// sweepInterval is the sweep tick period; falls back to config.DefaultSweepInterval.
	// The sweeper runs even when listenerTTL is zero: it also retries deferred callbacks.
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

	// The queue belongs to this stream, not the manager: a second listen() gets a fresh
	// one, so no callback can outlive the stream that produced it.
	q := make(chan handlerCall, n.handlerQueueSize)

	// Never cancelled, only deadline-bounded by callHandler: listen() often returns
	// *because* ctx was cancelled, and queued callbacks must not get a dead context.
	callCtx := context.WithoutCancel(ctx)

	// Workers are deliberately not in g: a listener that ignores cancellation never
	// returns, so g.Wait() never would either. The teardown below bounds them instead.
	// Ranging over q makes shutdown lossless -- close(q) ends the range only once the
	// buffer is empty -- and leaves no cancellable context to hand a listener.
	var pool sync.WaitGroup
	for range n.handlerWorkers {
		pool.Go(func() {
			for c := range q {
				n.callHandler(callCtx, c)
			}
		})
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
	g.Go(func() error { return n.runDispatcher(gCtx, q) })

	err = g.Wait()
	logger.Debugf("Notification listener stream stopped.")

	// One deadline for the whole teardown, however many listeners are pending: settle
	// what is left into the queue, close it, let the workers drain the backlog. A
	// listener that never returns keeps its worker; we abandon the wait rather than hang
	// listen(), and a later listen() starts with its own queue and workers.
	settled := make(chan struct{})
	go func() {
		defer close(settled)
		n.settleAllAndClear(q, fdriver.Unknown)
		close(q)
		pool.Wait()
	}()

	select {
	case <-settled:
		logger.Debugf("All finality handler callbacks finished.")
	case <-time.After(n.handlerTimeout):
		logger.Warnf(
			"Abandoning finality handler teardown after %s; a listener is not returning.",
			n.handlerTimeout)
	}

	return err
}

// runDispatcher settles notifications and sweeps due entries until gCtx is done.
//
// Sweeping from this goroutine rather than a dedicated one makes double-invoke
// impossible rather than merely guarded: dispatch and sweepExpired both settle by
// deleting an entry, and handlersMu alone would not stop one taking a listener the
// other already holds. It is also what lets handOff trust cap-len.
func (n *notificationListenerManager) runDispatcher(gCtx context.Context, q chan handlerCall) error {
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
			n.sweepExpired(q)
		case resp := <-n.responseQueue:
			n.dispatch(q, resp)
		}
	}
}

// handOff queues one entry's listeners, all of them or none, and reports which.
//
// Never blocks: the caller is the dispatcher, which also drives the sweeper. cap-len
// is a trustworthy lower bound because workers only consume, and the three producers
// -- dispatch, sweepExpired, settleAllAndClear -- never run concurrently.
//
// All-or-nothing keeps the caller to one decision; a partial hand-off would have to
// remember which listeners already ran. An entry with more listeners than the queue's
// whole capacity never fits here, and is settled at teardown where sends block.
func handOff(q chan handlerCall, txID string, listeners []fabric.FinalityListener, status int) bool {
	if cap(q)-len(q) < len(listeners) {
		return false
	}
	for _, h := range listeners {
		q <- handlerCall{handler: h, txID: txID, status: status}
	}
	return true
}

// logDeferred emits one aggregated warning per batch: a full queue affects every
// remaining entry, so per-txID warnings would flood the log at the notification rate.
func (n *notificationListenerManager) logDeferred(deferred, total int) {
	if deferred == 0 {
		return
	}
	logger.Warnf(
		"deferred %d of %d finality notifications: callback queue full with %d handler workers. "+
			"Either listeners are not keeping up (raise handlerWorkers), one is ignoring its "+
			"context, or this batch was larger than the queue. The affected listeners stay "+
			"registered with the status received and are retried on the next sweep.",
		deferred, total, n.handlerWorkers)
}

// settleAllAndClear empties the handlers map into q, so every listener still waiting
// is settled rather than dropped. Used on stream teardown.
//
// An entry with a remembered status is settled with it rather than downgraded; the
// status argument is the fallback for entries nothing was heard for. Sends block
// here, unlike the live paths: there is no next sweep to defer to, and listen()
// already bounds the whole teardown.
func (n *notificationListenerManager) settleAllAndClear(q chan handlerCall, status int) {
	var calls []handlerCall

	n.handlersMu.Lock()
	for txID, entry := range n.handlers {
		settleWith := status
		if entry.status != nil {
			settleWith = *entry.status
		}
		for _, h := range entry.listeners {
			calls = append(calls, handlerCall{handler: h, txID: txID, status: settleWith})
		}
	}
	clear(n.handlers)
	n.handlersMu.Unlock()

	if len(calls) == 0 {
		logger.Debugf("Cleared handlers map on listen() exit")
		return
	}

	logger.Debugf("Settling %d pending finality listener(s) on stream teardown", len(calls))
	for _, c := range calls {
		q <- c
	}
}

// callHandler invokes one listener synchronously under a handlerTimeout deadline. The
// timeout is advisory: it cancels the context but cannot force a return, so a listener
// that ignores it occupies this worker for as long as it runs.
func (n *notificationListenerManager) callHandler(ctx context.Context, c handlerCall) {
	timeoutCtx, cancel := context.WithTimeout(ctx, n.handlerTimeout)
	defer cancel()

	// Warn from the deadline itself, not on the way out. A check after OnStatus returns
	// can only ever fire for a listener that eventually came back, so the case actually
	// worth reporting -- one that never returns and holds its worker for good -- was
	// silent, which is also the line configuration.md tells operators to look for.
	//
	// ctx is never cancelled (see listen), so the deadline is the only thing that can
	// trigger this, and stop() runs before cancel() below, so our own cleanup cannot.
	stop := context.AfterFunc(timeoutCtx, func() {
		logger.Warnf(
			"OnStatus handler for txID=%s has not returned within %s and is holding one of "+
				"%d handler workers; OnStatus must observe ctx.Done() and return promptly",
			c.txID, n.handlerTimeout, n.handlerWorkers)
	})
	defer stop()

	c.handler.OnStatus(timeoutCtx, c.txID, c.status, "")
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
// Deletes only what was handed off: a listener deleted after a failed hand-off would
// be in neither the queue nor the map, and the sweeper only scans the map. A deferred
// entry keeps its listeners, its deadline and the status the committer sent, so the
// next sweep delivers the real answer rather than Unknown.
//
// Takes no context; callbacks run under the pool's context.
func (n *notificationListenerManager) dispatch(q chan handlerCall, resp *committerpb.NotificationResponse) {
	deferred, total := 0, 0

	n.handlersMu.Lock()
	for txID, status := range parseResponse(resp) {
		entry, ok := n.handlers[txID]
		if !ok {
			continue
		}
		total++

		if !handOff(q, txID, entry.listeners, status) {
			deferred++
			entry.status = &status
			continue
		}
		delete(n.handlers, txID)
	}
	n.handlersMu.Unlock()

	n.logDeferred(deferred, total)
}

// sweepExpired settles the entries that are due: those carrying a status the queue was
// too full to deliver, and those whose local deadline has passed.
//
// Without the expiry half, only an inbound notification ever removes an entry, so a
// committer that never reports on a transaction leaves the entry, and the listener
// closure it pins, in the map forever. The outbound request timeout does not help: it
// asks the *committer* to reply, so it depends on the stream we stopped hearing from.
//
// An expired entry is settled with Unknown, matching the committer's own TimeoutTxIds
// path. That can report Unknown for a transaction that did commit, since the remote
// timeout is non-strict; listenerTTL sits well above requestTimeout to make it
// unlikely. A deferred entry is different -- its answer already arrived, so it is
// retried with that status. That is why the sweeper runs even when listenerTTL is
// zero: the setting disables the backstop, not redelivery of a result we were given.
//
// An entry that still does not fit stays as it is and comes round on the next tick.
// Runs on the dispatcher goroutine, and takes no context; see runDispatcher.
func (n *notificationListenerManager) sweepExpired(q chan handlerCall) {
	now := time.Now()
	deferred, total := 0, 0

	n.handlersMu.Lock()
	for txID, entry := range n.handlers {
		// A remembered status means the answer did arrive and only the hand-off
		// failed; report that rather than Unknown, and do so whatever the deadline
		// says. Otherwise the deadline decides, and only while local expiry is on.
		status := fdriver.Unknown
		switch {
		case entry.status != nil:
			status = *entry.status
		case n.listenerTTL > 0 && !entry.expiresAt.IsZero() && !entry.expiresAt.After(now):
		default:
			continue
		}
		total++

		if !handOff(q, txID, entry.listeners, status) {
			deferred++
			continue
		}
		delete(n.handlers, txID)
	}
	n.handlersMu.Unlock()

	if total == 0 {
		return
	}

	logger.Debugf("Settling %d due finality listener entr(ies)", total)
	n.logDeferred(deferred, total)
}
