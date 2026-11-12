/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package views holds the initiator view of the endorsers suites: an app that spreads its
// transactions over several endorser nodes, any one of which satisfies the namespace's
// endorsement policy, and moves on to the next one when an endorser is unavailable. The
// Fabric and Fabric-x suites share it. The transaction, its ordering and the responder are
// those of the faultrecovery views.
package views

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"syscall"
	"time"

	fviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Endorser is an endorser node EndorseAnyView may ask.
type Endorser struct {
	// Name is the FSC node name, which EndorseAnyView reports.
	Name     string
	Identity view.Identity
}

// EndorseAny is the input of EndorseAnyView.
type EndorseAny struct {
	// Value is stored in the created Asset.
	Value int
	// Endorsers are the endorser nodes to pick from, in round-robin order.
	Endorsers []Endorser
	// Attempt bounds each attempt on one endorser, so a hung endorser costs the
	// initiator Attempt rather than the view's one-minute timer.
	Attempt time.Duration
	// Cooldown is how long an endorser that failed is asked only after the others.
	Cooldown time.Duration
}

// Attempt is an endorser EndorseAnyView asked without getting its endorsement.
type Attempt struct {
	Endorser string
	// Err is the error the attempt ended with.
	Err string
}

// Endorsed is what EndorseAnyView returns.
type Endorsed struct {
	// Tx is the serialized transaction, the input of the faultrecovery OrderView.
	Tx   []byte
	TxID string
	// Endorser is the node whose endorsement the transaction carries.
	Endorser string
	// Failed are the endorsers asked before Endorser, in order. Each received the
	// transaction, or was meant to.
	Failed []Attempt
}

// EndorseAnyView creates an Asset and collects one endorsement for it. It asks the
// endorsers one at a time, starting with the next one in round-robin order and leaving
// those that failed within Cooldown for last. When an endorser is unavailable, it passes
// the same transaction to the next one: an endorsement that failed before it verified
// added nothing to it. An endorser is unavailable when it rejects the session because it
// is draining or busy, the session closes, it does not answer within Attempt, or it cannot
// be dialed: the connection is refused, or the handshake of a new connection outlasts
// Attempt. Any other error fails the view.
type EndorseAnyView struct {
	EndorseAny
	selector *selector
}

func (v *EndorseAnyView) Call(viewCtx view.Context) (any, error) {
	if len(v.Endorsers) == 0 {
		return nil, errors.New("no endorsers")
	}
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	tx.SetNamespace(fviews.Namespace)
	if err := tx.AddOutput(&fviews.Asset{ID: tx.ID(), Value: v.Value}); err != nil {
		return nil, err
	}
	// The initiator endorses first, which signs the proposal the endorser then endorses:
	// a transaction with endorsements but without the proposal they endorse cannot be
	// ordered. The policy names only the endorsers' organization, so the transaction
	// still needs one endorser's endorsement.
	me := tx.FabricNetworkService().IdentityProvider().DefaultIdentity()
	if _, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, me)); err != nil {
		return nil, err
	}

	res := &Endorsed{TxID: tx.ID()}
	for _, e := range v.selector.order(v.Endorsers, v.Cooldown) {
		err := v.collect(viewCtx, tx, e.Identity)
		v.selector.report(e.Name, err)
		if err == nil {
			if res.Tx, err = tx.Bytes(); err != nil {
				return nil, err
			}
			res.Endorser = e.Name
			return res, nil
		}
		if !unavailable(err) {
			return nil, errors.WithMessagef(err, "endorser [%s] failed tx [%s]", e.Name, tx.ID())
		}
		res.Failed = append(res.Failed, Attempt{Endorser: e.Name, Err: err.Error()})
	}
	return nil, errors.Errorf("no endorser endorsed tx [%s]: %v", tx.ID(), res.Failed)
}

// collect asks the endorser id for its endorsement of tx, for at most v.Attempt.
func (v *EndorseAnyView) collect(viewCtx view.Context, tx *state.Transaction, id view.Identity) error {
	ctx, cancel := context.WithTimeout(viewCtx.Context(), v.Attempt)
	defer cancel()
	_, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, id), view.WithGoContext(ctx))
	return err
}

// unavailable reports whether err, from collecting an endorsement, says that the endorser
// could not endorse at the moment, so another one may.
func unavailable(err error) bool {
	// A deadline shows as context.DeadlineExceeded once a session is open, and as a
	// socket timeout, os.ErrDeadlineExceeded, while a new connection is still dialing.
	for _, target := range []error{session.ErrDraining, session.ErrBusy, comm.ErrSessionClosed, context.DeadlineExceeded, os.ErrDeadlineExceeded, syscall.ECONNREFUSED} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// selector keeps, across the calls of a node's EndorseAnyView, the round-robin position
// and when each endorser last failed.
type selector struct {
	mu     sync.Mutex
	next   int
	failed map[string]time.Time
}

// order returns endorsers in round-robin order, starting one further than the previous
// call, with those that failed within cooldown moved to the end.
func (s *selector) order(endorsers []Endorser, cooldown time.Duration) []Endorser {
	s.mu.Lock()
	defer s.mu.Unlock()
	start := s.next % len(endorsers)
	s.next++
	var ready, cooling []Endorser
	for i := range endorsers {
		e := endorsers[(start+i)%len(endorsers)]
		if at, ok := s.failed[e.Name]; ok && time.Since(at) < cooldown {
			cooling = append(cooling, e)
		} else {
			ready = append(ready, e)
		}
	}
	return append(ready, cooling...)
}

// report records the outcome err of an attempt on the endorser name.
func (s *selector) report(name string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		delete(s.failed, name)
		return
	}
	if s.failed == nil {
		s.failed = map[string]time.Time{}
	}
	s.failed[name] = time.Now()
}

// EndorseAnyViewFactory makes the EndorseAnyViews of one node, which share its selector.
type EndorseAnyViewFactory struct {
	selector selector
}

func (f *EndorseAnyViewFactory) NewView(in []byte) (view.View, error) {
	v := &EndorseAnyView{selector: &f.selector}
	if err := json.Unmarshal(in, &v.EndorseAny); err != nil {
		return nil, err
	}
	return v, nil
}
