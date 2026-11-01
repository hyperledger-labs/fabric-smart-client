/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Rejected is what LiarEndorseView returns when the initiator rejects the responder's
// endorsement.
type Rejected struct {
	// TxID is the ID of the rejected transaction.
	TxID string
	// Responder is the name of the responder whose endorsement was rejected.
	Responder string
	// Reason is the error the endorsement collection failed with.
	Reason string
}

// LiarEndorseView creates an Asset and collects endorsements for it from a responder
// running LiarResponderView. It returns Rejected when the collection fails, and an error
// when the collection succeeds, since an endorsement over changed results must be
// rejected. It has its own type, so LiarResponderView can be registered for it without
// replacing the honest responder of EndorseView.
type LiarEndorseView struct {
	Submit
}

func (v *LiarEndorseView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	tx.SetNamespace(Namespace)
	if err := tx.AddOutput(&Asset{ID: tx.ID(), Value: v.Value}); err != nil {
		return nil, err
	}

	me := tx.FabricNetworkService().IdentityProvider().DefaultIdentity()
	_, err = viewCtx.RunView(state.NewCollectEndorsementsView(tx, me, v.Responder))
	if err == nil {
		return nil, errors.Errorf("tx [%s]: endorsement over changed results was accepted", tx.ID())
	}
	resolver, rerr := endpoint.GetService(viewCtx).GetResolver(viewCtx.Context(), v.Responder)
	if rerr != nil {
		return nil, errors.WithMessagef(rerr, "failed resolving responder of rejected tx [%s]", tx.ID())
	}
	return &Rejected{TxID: tx.ID(), Responder: resolver.GetName(), Reason: err.Error()}, nil
}

// LiarEndorseViewFactory instantiates LiarEndorseView from JSON input.
type LiarEndorseViewFactory struct{}

func (*LiarEndorseViewFactory) NewView(in []byte) (view.View, error) {
	f := &LiarEndorseView{}
	if err := json.Unmarshal(in, &f.Submit); err != nil {
		return nil, err
	}
	return f, nil
}

// LiarResponderView increments the value of the proposed transaction's Asset before
// endorsing it, acting as a counterparty that tampers with the read-write set.
type LiarResponderView struct{}

func (*LiarResponderView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	asset := &Asset{}
	if err := tx.GetOutputAt(0, asset); err != nil {
		return nil, err
	}
	asset.Value++
	if err := tx.AddOutput(asset); err != nil {
		return nil, err
	}
	return viewCtx.RunView(state.NewEndorseView(tx))
}
