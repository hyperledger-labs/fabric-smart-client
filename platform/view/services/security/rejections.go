/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package security records data a node refused from remote peers because it
// failed a security check.
package security

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

// Kind names the security check a rejected input failed. It is the value of
// the kind label of the rejections counter.
type Kind string

const (
	// KindInvalidTx is a received transaction that failed validation.
	KindInvalidTx Kind = "invalid_tx"
	// KindBindingRefused is a binding the binding store refused to store.
	KindBindingRefused Kind = "binding_refused"
	// KindBindingConflict is a binding that conflicts with an existing one.
	KindBindingConflict Kind = "binding_conflict"
	// KindRecipientProof is a recipient identity without a valid proof of ownership.
	KindRecipientProof Kind = "recipient_proof"
	// KindUnauthorized is a first message whose sender a responder's authorizer refused.
	KindUnauthorized Kind = "unauthorized"
	// KindStoreConflict is a write that conflicts with the value a write-once
	// store already holds under the same key.
	KindStoreConflict Kind = "store_conflict"
)

// logInterval is the minimum time between two warn lines for the same kind.
const logInterval = time.Second

// Rejections counts security rejections by kind and logs them at warn level.
// Remote peers control the rate of rejections, so it writes at most one warn
// line per kind per second; the counter counts every rejection. It is safe for
// concurrent use.
type Rejections struct {
	counter metrics.Counter

	mu     sync.Mutex
	logged map[Kind]time.Time

	// now and warnf are replaced by tests.
	now   func() time.Time
	warnf func(ctx context.Context, template string, args ...any)
}

// NewRejections returns a Rejections that reports its counter to p. A nil p
// discards the counts.
func NewRejections(p metrics.Provider) *Rejections {
	if p == nil {
		p = &disabled.Provider{}
	}
	return &Rejections{
		counter: p.NewCounter(metrics.CounterOpts{
			Name:       "rejections",
			Help:       "The number of inputs from remote peers rejected by a security check, by kind",
			LabelNames: []string{"kind"},
		}),
		logged: map[Kind]time.Time{},
		now:    time.Now,
		warnf:  logger.WarnfContext,
	}
}

// Report records that input from peer, for transaction txID, failed the
// security check kind for reason. txID may be empty; the warn line quotes it.
// reason must not contain attacker-controlled bytes beyond the txID. On a nil
// Rejections it reports to the shared instance that discards its counts.
func (r *Rejections) Report(ctx context.Context, kind Kind, peer view.Identity, txID, reason string) {
	if r == nil {
		r = fallback
	}
	r.counter.With("kind", string(kind)).Add(1)

	r.mu.Lock()
	now := r.now()
	last, ok := r.logged[kind]
	shouldLog := !ok || now.Sub(last) >= logInterval
	if shouldLog {
		r.logged[kind] = now
	}
	r.mu.Unlock()

	if shouldLog {
		r.warnf(ctx, "security rejection [%s] from peer [%s] for tx %q: %s", kind, peer, txID, reason)
	}
}

// fallback is shared by every GetRejections call that finds no registered
// instance, so its warn-log rate limit holds across those callers.
var fallback = NewRejections(nil)

// GetRejections returns the Rejections registered in sp, or a shared instance
// that discards its counts when none is registered, so callers can always report.
func GetRejections(sp services.Provider) *Rejections {
	if s, err := sp.GetService(reflect.TypeFor[*Rejections]()); err == nil {
		if r, ok := s.(*Rejections); ok {
			return r
		}
	}
	return fallback
}
