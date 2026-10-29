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

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
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
	// newNotifyClient returns the stub to open a stream on. stream calls it once per
	// attempt, so each reconnect can land on a different endpoint (see config.ClientProvider).
	newNotifyClient func() (committerpb.SidecarServiceClient, string, error)
	requestQueue    chan *committerpb.NotificationRequest
	responseQueue   chan *committerpb.NotificationResponse
	handlerTimeout  time.Duration

	// handlerWorkers is how many OnStatus callbacks may run at once, and so also how
	// many stuck listeners it takes to stop delivering notifications on this manager.
	handlerWorkers int

	// handlerQueueSize is the capacity of the callback queue serve creates. It absorbs
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

	// streamCtx holds the context of the current notification stream, if one has been
	// opened. Its Done() channel closes as soon as that stream fails or the manager
	// stops, and stays closed until the next stream replaces it. AddFinalityListener
	// selects on it so a send to requestQueue never blocks on a stream that is gone.
	streamCtx atomic.Pointer[context.Context]

	// reconnectMinDelay and reconnectMaxDelay bound the backoff between stream
	// attempts; zero falls back to defaultReconnectMinDelay and defaultReconnectMaxDelay.
	reconnectMinDelay, reconnectMaxDelay time.Duration

	// stopped is set, under handlersMu, before serve returns, once nothing will settle
	// a new entry. AddFinalityListener rejects registrations from then on.
	stopped bool

	// newConn returns the tracker of one endpoint; conn is the tracker of connEndpoint,
	// the endpoint of the current stream. A nil conn reports nothing. Only the goroutine
	// running run touches them.
	newConn      func(endpoint string) *connstate.Tracker
	conn         *connstate.Tracker
	connEndpoint string
}

// Default reconnect backoff bounds. The delay is jittered and doubles per failed
// attempt up to the maximum. It starts over after a stream that stayed up longer than
// the maximum, so a committer that accepts and then drops every stream at once is still
// retried with backoff.
const (
	defaultReconnectMinDelay = 100 * time.Millisecond
	defaultReconnectMaxDelay = 10 * time.Second
)

// maxTxIDsPerRequest caps the txIDs re-requested in one NotificationRequest. It
// matches the committer's default per-request limit, above which the committer
// rejects the whole request.
const maxTxIDsPerRequest = 1000

// run serves the manager until ctx ends, reopening the notification stream with
// backoff whenever it fails. Pending listeners survive a stream failure: the next
// stream requests their txIDs again, and the sweeper keeps expiring them in the
// meantime. Only the end of ctx settles what is left, with Unknown.
//
// The stream is reported connecting before the first attempt, connected once a stream
// opens, reconnecting before each later attempt, and disconnected once ctx ends. Each
// endpoint has its own series; see track.
func (n *notificationListenerManager) run(ctx context.Context) error {
	minDelay, maxDelay := n.reconnectMinDelay, n.reconnectMaxDelay
	if minDelay <= 0 {
		minDelay = defaultReconnectMinDelay
	}
	if maxDelay <= 0 {
		maxDelay = defaultReconnectMaxDelay
	}
	return n.serve(ctx, func(ctx context.Context) error {
		defer func() { n.conn.Set(connstate.Disconnected) }() // the tracker current at return
		backoff := utils.NewBackoffRetryRunner(utils.Infinitely, minDelay, maxDelay)
		for {
			start := time.Now()
			err := n.stream(ctx)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if time.Since(start) > maxDelay {
				backoff = utils.NewBackoffRetryRunner(utils.Infinitely, minDelay, maxDelay)
			}
			delay := backoff.NextDelay()
			logger.Warnf("Notification stream failed, reconnecting in %s: %v", delay, err)
			n.conn.Set(connstate.Reconnecting)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	})
}

// serve runs the dispatcher, the sweeper and the handler pool until connect returns or
// ctx ends, then stops the manager: every listener still pending is settled, with the
// status the committer sent if one arrived and Unknown otherwise.
func (n *notificationListenerManager) serve(ctx context.Context, connect func(context.Context) error) error {
	// One group for the connection and the dispatcher, so either ending stops both.
	g, gCtx := errgroup.WithContext(ctx)

	// The queue belongs to this serve call, not the manager, so no callback can outlive
	// the call that produced it.
	q := make(chan handlerCall, n.handlerQueueSize)

	// Never cancelled, only deadline-bounded by callHandler: serve often returns
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

	g.Go(func() error { return connect(gCtx) })
	g.Go(func() error { return n.runDispatcher(gCtx, q) })

	err := g.Wait()
	logger.Debugf("Notification listener manager stopped.")

	// Before serve can return, so a registration made after it is rejected rather
	// than left in a map nothing will ever settle.
	n.handlersMu.Lock()
	n.stopped = true
	n.handlersMu.Unlock()

	// One deadline for the whole teardown, however many listeners are pending: settle
	// what is left into the queue, close it, let the workers drain the backlog. A
	// listener that never returns keeps its worker; we abandon the wait rather than hang
	// serve.
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

// stream opens one notification stream, requests every pending txID on it, and then
// relays requests and responses until the stream fails or ctx ends.
func (n *notificationListenerManager) stream(ctx context.Context) error {
	logger.Debugf("Notification listener stream starting.")
	notifyClient, endpoint, err := n.newNotifyClient()
	if err != nil {
		return err
	}
	n.track(endpoint)
	// The stream runs on gCtx so that the first goroutine to fail also cancels the
	// gRPC call, which unblocks the other one.
	g, gCtx := errgroup.WithContext(ctx)
	notifyStream, err := notifyClient.OpenNotificationStream(gCtx)
	if err != nil {
		_ = g.Wait() // releases gCtx; nothing was started on g
		return err
	}
	n.conn.Set(connstate.Connected)

	pending := n.attach(gCtx)

	// receiver
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

	// sender
	g.Go(func() error {
		for _, req := range pending {
			if err := notifyStream.Send(req); err != nil {
				return err
			}
		}
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

	err = g.Wait()
	logger.Debugf("Notification listener stream stopped.")
	return err
}

// attach makes streamCtx the stream AddFinalityListener sends to, and returns the
// requests that subscribe that stream to every txID still waiting for an answer. The
// committer reports a transaction only to the subscription it was requested on, so a
// stream that replaces a failed one knows nothing of the txIDs requested before. An
// entry holding a remembered status already has its answer and is skipped.
//
// Both happen under one write lock: a registration either lands in the snapshot or
// sees the new stream and sends its own request, so none is missed. Taking the lock
// twice would let a registration see the new stream and block on requestQueue, still
// holding handlersMu, before the sender that drains it has started.
func (n *notificationListenerManager) attach(streamCtx context.Context) []*committerpb.NotificationRequest {
	n.handlersMu.Lock()
	n.streamCtx.Store(&streamCtx)
	txIDs := make([]string, 0, len(n.handlers))
	for txID, entry := range n.handlers {
		if entry.status == nil {
			txIDs = append(txIDs, txID)
		}
	}
	n.handlersMu.Unlock()

	var reqs []*committerpb.NotificationRequest
	for batch := range slices.Chunk(txIDs, maxTxIDsPerRequest) {
		reqs = append(reqs, n.newRequest(batch...))
	}
	return reqs
}

// newRequest subscribes to txIDs. Its Timeout tells the committer to reply once it
// passes rather than leaving it to its own internal max-timeout. Without it, a
// client-side abort (e.g. our own listenerTTL firing) can mark transactions Unknown
// that the committer already knows the outcome of, because the committer never got a
// reason to answer early. See notify.proto's Timeout field doc.
func (n *notificationListenerManager) newRequest(txIDs ...string) *committerpb.NotificationRequest {
	return &committerpb.NotificationRequest{
		TxStatusRequest: &committerpb.TxIDsBatch{TxIds: txIDs},
		Timeout:         durationpb.New(n.requestTimeout),
	}
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
// is settled rather than dropped. Used on teardown.
//
// An entry with a remembered status is settled with it rather than downgraded; the
// status argument is the fallback for entries nothing was heard for. Sends block
// here, unlike the live paths: there is no next sweep to defer to, and serve
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
		logger.Debugf("Cleared handlers map on manager stop")
		return
	}

	logger.Debugf("Settling %d pending finality listener(s) on manager stop", len(calls))
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
	// ctx is never cancelled (see serve), so the deadline is the only thing that can
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

// track moves state reporting to endpoint before a stream is opened on it. As in the
// delivery service, the series follows the endpoint: the first one is reported connecting,
// and a different one gets its own series, reported reconnecting, while the previous one
// is reported disconnected. The same endpoint keeps the reconnecting state run set.
func (n *notificationListenerManager) track(endpoint string) {
	if n.conn != nil && endpoint == n.connEndpoint {
		return
	}
	s := connstate.Reconnecting
	if n.conn == nil {
		s = connstate.Connecting
	}
	n.conn.Set(connstate.Disconnected) // nil-safe
	if n.newConn != nil {
		n.conn = n.newConn(endpoint)
	}
	n.connEndpoint = endpoint
	n.conn.Set(s)
}

func parseResponse(resp *committerpb.NotificationResponse) map[string]int {
	res := make(map[string]int)

	// first parse all timeouts
	for _, txID := range resp.GetTimeoutTxIds() {
		res[txID] = fdriver.Unknown
	}

	// The committer never reports on a txID it rejected (over its per-request or
	// active-subscription limit), so it is settled Unknown, like a timeout, rather than
	// left waiting for listenerTTL.
	if rejected := resp.GetRejectedTxIds(); rejected != nil {
		logger.Warnf("committer rejected %d finality subscription(s): %s", len(rejected.GetTxIds()), rejected.GetReason())
		for _, txID := range rejected.GetTxIds() {
			res[txID] = fdriver.Unknown
		}
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

// AddFinalityListener registers a listener to be notified when the transaction with the
// given txID reaches finality. While the notification stream is down the registration
// is kept and requested once the stream is back. It fails only for invalid arguments or
// once the manager has stopped.
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

	if n.stopped {
		return errors.Errorf("notification stream unavailable, cannot register listener for txID=%s", txID)
	}

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

	// Without a live stream the entry just waits: the next stream requests every
	// pending txID when it opens (see stream), and the sweeper expires the entry if
	// that takes longer than listenerTTL. handlersMu, held since the top of this call,
	// is what keeps this in step with that snapshot.
	sc := n.streamCtx.Load()
	if sc == nil {
		return nil
	}
	select {
	case n.requestQueue <- n.newRequest(txID):
	case <-(*sc).Done():
	}
	return nil
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
