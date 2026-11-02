/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ForgedTxIDView creates an Asset in a transaction whose txID is not derived from its
// nonce and creator, and collects endorsements for it from a responder running the
// honest ResponderView. The txID is that of a transaction with another nonce, as a
// sender would choose to make the responder store the transaction under another
// transaction's ID. It returns Rejected when the collection fails, and an error when
// the collection succeeds. It has its own type, so ResponderView can be registered for
// it next to EndorseView.
type ForgedTxIDView struct {
	Submit
}

func (v *ForgedTxIDView) Call(viewCtx view.Context) (any, error) {
	fns, err := fabric.GetDefaultFNS(viewCtx)
	if err != nil {
		return nil, err
	}
	me := fns.IdentityProvider().DefaultIdentity()
	tm := fns.TransactionManager()
	// endorser.NewTransaction does not pass a txID on, so the transaction is built
	// through the transaction manager. A nil nonce makes ComputeTxID draw a fresh one.
	ftx, err := tm.NewTransaction(
		fabric.WithContext(viewCtx.Context()),
		fabric.WithCreator(me),
		fabric.WithTxID(tm.ComputeTxID(&fabric.TxID{Creator: me})),
	)
	if err != nil {
		return nil, err
	}
	etx := &endorser.Transaction{Provider: viewCtx, Transaction: ftx}
	defer etx.Close()
	tx, err := state.Wrap(etx)
	if err != nil {
		return nil, err
	}
	tx.SetNamespace(Namespace)
	if err := tx.AddOutput(&Asset{ID: tx.ID(), Value: v.Value}); err != nil {
		return nil, err
	}

	_, err = viewCtx.RunView(state.NewCollectEndorsementsView(tx, me, v.Responder))
	if err == nil {
		return nil, errors.Errorf("tx [%s]: transaction with a forged txID was endorsed", tx.ID())
	}
	resolver, rerr := endpoint.GetService(viewCtx).GetResolver(viewCtx.Context(), v.Responder)
	if rerr != nil {
		return nil, errors.WithMessagef(rerr, "failed resolving responder of rejected tx [%s]", tx.ID())
	}
	return &Rejected{TxID: tx.ID(), Responder: resolver.GetName(), Reason: err.Error()}, nil
}

// ForgedTxIDViewFactory instantiates ForgedTxIDView from JSON input.
type ForgedTxIDViewFactory struct{}

func (*ForgedTxIDViewFactory) NewView(in []byte) (view.View, error) {
	f := &ForgedTxIDView{}
	if err := json.Unmarshal(in, &f.Submit); err != nil {
		return nil, err
	}
	return f, nil
}
