/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Namespace is where the market's states live on the ledger.
const Namespace = "trading"

// newTransaction starts a transaction in the trading namespace with a single
// command and its parties.
func newTransaction(viewCtx view.Context, command string, parties ...view.Identity) (*state.Transaction, error) {
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating a new transaction")
	}
	tx.SetNamespace(Namespace)
	if err := tx.AddCommand(command, parties...); err != nil {
		return nil, errors.Wrapf(err, "failed adding command [%s]", command)
	}
	return tx, nil
}

// submit collects the endorsements of parties, in order, then sends the
// transaction for ordering and waits until it is committed. A party that is
// this node endorses locally.
func submit(viewCtx view.Context, tx *state.Transaction, parties ...view.Identity) error {
	if _, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, parties...)); err != nil {
		return errors.WithMessage(err, "failed collecting endorsements")
	}
	if _, err := viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, time.Minute)); err != nil {
		return errors.WithMessage(err, "failed ordering the transaction")
	}
	return nil
}

// myIdentity returns this node's Fabric identity: the one it owns goods and
// coins with, and signs with.
func myIdentity(viewCtx view.Context) (view.Identity, error) {
	fns, _, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the default network")
	}
	return fns.IdentityProvider().DefaultIdentity(), nil
}

// kvsOf returns this node's local key-value store. What is stored there never
// leaves the node.
func kvsOf(viewCtx view.Context) (*kvs.KVS, error) {
	s, err := viewCtx.GetService(reflect.TypeFor[*kvs.KVS]())
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the local store")
	}
	store, ok := s.(*kvs.KVS)
	if !ok {
		return nil, errors.Errorf("unexpected local store type [%T]", s)
	}
	return store, nil
}

// getState reads one committed state.
func getState(viewCtx view.Context, key string, st any) error {
	vault, err := state.GetVault(viewCtx)
	if err != nil {
		return errors.Wrap(err, "failed getting the vault")
	}
	if err := vault.GetState(viewCtx.Context(), Namespace, key, st); err != nil {
		return errors.Wrapf(err, "failed reading [%s]", key)
	}
	return nil
}

// QueryState is the input of StateView.
type QueryState struct {
	Key string
}

// StateView returns the committed state under a key, as JSON. Any node can run
// it: every state of the market is stored in clear.
type StateView struct {
	QueryState
}

func (q *StateView) Call(viewCtx view.Context) (any, error) {
	var raw json.RawMessage
	if err := getState(viewCtx, q.Key, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// StateViewFactory creates a StateView from a JSON QueryState.
type StateViewFactory struct{}

func (*StateViewFactory) NewView(in []byte) (view.View, error) {
	f := &StateView{}
	if err := json.Unmarshal(in, &f.QueryState); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}
