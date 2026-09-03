/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// TxStatusResolver reports the current validation status of transactions from an
// authoritative source.
//
// A transaction the source does not yet know about must be reported as [fdriver.Unknown]
// rather than as an error, so that callers can tell "not final yet" from "the source could
// not be reached". An implementation is responsible for bounding its own calls: ctx is
// passed on, but the production implementation -- the vault in
// platform/fabricx/core/vault -- reaches a query service whose methods bound themselves
// with their own requestTimeout and ignore ctx.
type TxStatusResolver interface {
	Statuses(ctx context.Context, txIDs ...driver.TxID) ([]driver.TxValidationStatus[fdriver.ValidationCode], error)
}

// Why this exists: the committer's notification stream reports a transaction once, so a
// listener registered after that point is told Unknown for a transaction already known to
// be Valid -- which [driver.FinalityListener] forbids ("or it is already valid or
// invalid"). Resolving before subscribing closes that gap, and treating the notification
// path's Unknown as a reason to ask again rather than as a verdict closes it for a
// listener that was subscribed before the answer arrived.

// ResolvingListenerManager is a [ListenerManager] that also answers for transactions whose
// status the committer has already reported. It is safe for concurrent use.
//
// Like the wrapped manager, it identifies a subscription by comparing listeners with ==,
// so a listener must be of a comparable type -- a pointer, in practice.
type ResolvingListenerManager struct {
	inner    ListenerManager
	resolver TxStatusResolver

	// proxies maps each live subscription to the proxy registered for it, so that
	// RemoveFinalityListener can unsubscribe the right one and AddFinalityListener can
	// recognise a duplicate. A listener settled at registration never appears here, and a
	// proxy drops its own entry as it delivers. The key carries the txID as well as the
	// listener because the same listener may be registered for more than one transaction;
	// keying on the listener alone would orphan the earlier subscription.
	mu      sync.Mutex
	proxies map[proxyKey]*proxyListener
}

// proxyKey identifies one subscription: the same listener may be registered for more
// than one transaction, and each of those has its own proxy.
type proxyKey struct {
	txID     driver.TxID
	listener fabric.FinalityListener
}

// NewResolvingListenerManager wraps inner so that an already-final transaction is answered
// from resolver instead of from a subscription the committer will not honour.
func NewResolvingListenerManager(inner ListenerManager, resolver TxStatusResolver) *ResolvingListenerManager {
	return &ResolvingListenerManager{
		inner:    inner,
		resolver: resolver,
		proxies:  map[proxyKey]*proxyListener{},
	}
}

// AddFinalityListener implements [ListenerManager]. If the transaction is already final,
// listener is invoked with that status on a separate goroutine after this call returns,
// and nothing is subscribed; otherwise listener is registered with the wrapped manager.
//
// Registering the same listener for the same txID twice is a no-op that reports success,
// matching the wrapped manager: a second proxy would deliver the caller's callback twice
// and leave behind a subscription RemoveFinalityListener cannot reach.
//
// It reports an error if txID is empty, if listener is nil, or if the wrapped manager
// rejects the registration. A resolver that cannot be reached is not an error: the
// registration falls back to the wrapped manager.
//
// The status query runs before this call returns, and [ListenerManager] has no ctx to
// bound it with, so a slow resolver delays every registration by its own timeout.
func (m *ResolvingListenerManager) AddFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error {
	// Same message as the inner manager and the generic committer, so the same API call
	// behaves the same way on every driver.
	if len(txID) == 0 {
		return errors.New("tx id must be not empty")
	}
	if listener == nil {
		return errors.New("listener nil")
	}

	if code, message, ok := m.resolve(context.Background(), txID); ok {
		logger.Debugf("txID=%s is already final [%d]; settling without subscribing", txID, code)
		// Off the caller's goroutine: a listener is entitled to block, and one that calls
		// back into this manager would deadlock on mu. The generic driver is asynchronous
		// here too -- its status poller invokes later, never inside registration.
		go listener.OnStatus(context.Background(), txID, code, message)
		return nil
	}

	key := proxyKey{txID: txID, listener: listener}

	// mu is held across the inner registration so that a concurrent
	// RemoveFinalityListener for the same key either finds no subscription yet or finds
	// the proxy. Releasing mu in between would let it find nothing, report success, and
	// leave the proxy subscribed with no way to reach it. Nothing on the inner path calls
	// back into this manager, so holding mu here cannot deadlock; the inner manager holds
	// its own map lock across the same registration for the same reason.
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, duplicate := m.proxies[key]; duplicate {
		logger.Warnf("The exact same listener is already registered for txID=%v. Skipping.", txID)
		return nil
	}

	p := &proxyListener{mgr: m, key: key, real: listener}
	if err := m.inner.AddFinalityListener(txID, p); err != nil {
		return err
	}
	m.proxies[key] = p
	return nil
}

// RemoveFinalityListener implements [ListenerManager]. Removing a listener that never
// subscribed -- because it was settled at registration -- or one that has already been
// delivered to, succeeds without touching the wrapped manager.
func (m *ResolvingListenerManager) RemoveFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error {
	key := proxyKey{txID: txID, listener: listener}

	m.mu.Lock()
	p, subscribed := m.proxies[key]
	delete(m.proxies, key)
	m.mu.Unlock()

	if !subscribed {
		return nil
	}
	return m.inner.RemoveFinalityListener(txID, p)
}

// resolve reports txID's status when the resolver says it is already final. ok is false
// both when the transaction is not final yet and when the resolver could not be reached:
// each means "subscribe and wait", so a transient query failure degrades to the
// notification path rather than failing a registration.
func (m *ResolvingListenerManager) resolve(ctx context.Context, txID driver.TxID) (fdriver.ValidationCode, string, bool) {
	statuses, err := m.resolver.Statuses(ctx, txID)
	if err != nil {
		logger.Debugf("could not resolve status for txID=%s, falling back to notifications: %v", txID, err)
		return fdriver.Unknown, "", false
	}
	if len(statuses) == 0 {
		return fdriver.Unknown, "", false
	}

	st := statuses[0]
	if st.ValidationCode == fdriver.Valid || st.ValidationCode == fdriver.Invalid {
		return st.ValidationCode, st.Message, true
	}
	return fdriver.Unknown, "", false
}

// proxyListener sits between the wrapped manager and the caller's listener, so that an
// Unknown from the notification path can be re-resolved rather than reported, and so that
// a delivered subscription stops being tracked.
type proxyListener struct {
	mgr  *ResolvingListenerManager
	key  proxyKey
	real fabric.FinalityListener
}

// OnStatus implements [fabric.FinalityListener]. An Unknown is re-resolved before being
// passed on; every other status is forwarded as it arrived.
//
// The re-resolve runs on the calling goroutine rather than on its own. The wrapped manager
// settles every pending listener with Unknown when its stream tears down or an entry
// expires, so a goroutine per listener would fan out one query per pending transaction at
// exactly the moment the committer is unreachable. Staying on the caller keeps the queries
// bounded by the wrapped manager's handler pool, which is what that pool is for, and keeps
// its teardown deadline meaningful. The cost is one query per listener instead of one
// batched query, and that a slow resolver is reported against this callback rather than
// against the listener behind it (see handlerTimeout in nlm.go).
func (p *proxyListener) OnStatus(ctx context.Context, txID driver.TxID, vc fdriver.ValidationCode, message string) {
	// The wrapped manager removes an entry as it settles it, so this subscription is
	// already gone. Dropping the bookkeeping here rather than leaving it to
	// RemoveFinalityListener is what keeps a caller that registers, is called back once
	// and moves on -- the documented pattern, since a listener is auto-removed when
	// invoked -- from leaking an entry per transaction.
	p.forget()

	if vc == fdriver.Unknown {
		if code, msg, ok := p.mgr.resolve(ctx, txID); ok {
			logger.Debugf("txID=%s reported Unknown but resolved to [%d]", txID, code)
			vc, message = code, msg
		}
	}
	p.real.OnStatus(ctx, txID, vc, message)
}

// forget stops tracking p's subscription, unless the caller has since registered the same
// listener for the same transaction again and p is no longer that key's live proxy.
func (p *proxyListener) forget() {
	p.mgr.mu.Lock()
	defer p.mgr.mu.Unlock()

	if p.mgr.proxies[p.key] == p {
		delete(p.mgr.proxies, p.key)
	}
}
