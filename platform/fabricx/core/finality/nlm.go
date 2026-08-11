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

// DefaultListenerTTL bounds how long a listener may wait for a notification that
// may never arrive. It is deliberately much longer than the timeout we ask the
// committer for in AddFinalityListener: that timeout is documented non-strict
// ("it is possible to receive notifications after the timeout has passed", see
// notify.proto), so the remote must be given ample room to answer before we give
// up locally. Expiry is a backstop against silence, not a competitor to the
// remote deadline.
const DefaultListenerTTL = 2 * time.Minute

// DefaultSweepInterval is how often expired entries are collected. An entry's
// worst-case lifetime is DefaultListenerTTL + DefaultSweepInterval.
const DefaultSweepInterval = 30 * time.Second

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

	// listenerTTL is how long an entry may stay unresolved before the sweeper
	// settles it with Unknown. Zero disables local expiry entirely.
	listenerTTL time.Duration
	// sweepInterval is the sweep tick period. Ignored when listenerTTL is zero;
	// falls back to DefaultSweepInterval if unset.
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
		// Sweep from this goroutine rather than a dedicated one: it is already the
		// only goroutine that deletes entries on the notification path, so a sweep
		// can never interleave with a dispatch and no listener can be settled twice.
		sweepEvery := n.sweepInterval
		if sweepEvery <= 0 {
			sweepEvery = DefaultSweepInterval
		}
		ticker := time.NewTicker(sweepEvery)
		defer ticker.Stop()

		for {
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case <-ticker.C:
				n.sweepExpired(gCtx)
			case resp := <-n.responseQueue:
				n.dispatch(gCtx, resp)
			}
		}
	})

	err = g.Wait()
	logger.Debugf("Notification listener stream stopped.")

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
			n.invokeHandler(ctx, h, p.txID, status)
		}
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
		// Timeout deliberately left unset: notify.proto has the committer apply
		// its own configured default when this field is absent.
		// The committer operator is in a better position to know
		// the right timeout for their network than FSC's client code is
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

// invokeHandler calls one listener in its own goroutine, bounded by
// handlerTimeout. If a handler ignores context cancellation and never returns,
// its goroutine leaks -- which is preferable to blocking the dispatcher. Shared
// by the notification path and the expiry sweeper so both get the same isolation.
func (n *notificationListenerManager) invokeHandler(ctx context.Context, h fabric.FinalityListener, txID string, status int) {
	go func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, n.handlerTimeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			h.OnStatus(timeoutCtx, txID, status, "")
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
func (n *notificationListenerManager) dispatch(ctx context.Context, resp *committerpb.NotificationResponse) {
	type handlerCall struct {
		handler fabric.FinalityListener
		txID    string
		status  int
	}

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

	for _, c := range calls {
		n.invokeHandler(ctx, c.handler, c.txID, c.status)
	}
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
// given up; DefaultListenerTTL is set well above the request timeout to make that
// unlikely. Callers needing certainty can query the transaction status directly.
//
// Runs on the dispatcher goroutine (see the ticker in listen): the dispatcher is
// the only other goroutine that deletes entries on the notification path, so a
// sweep and a dispatch can never interleave and one listener can never be settled
// twice. Keep it that way -- moving this off the dispatcher would make
// double-invoke merely preventable rather than impossible.
func (n *notificationListenerManager) sweepExpired(ctx context.Context) {
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

	for _, e := range batch {
		for _, h := range e.listeners {
			n.invokeHandler(ctx, h, e.txID, fdriver.Unknown)
		}
	}
}
