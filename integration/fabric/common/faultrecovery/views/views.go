/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package views holds the two-party transaction the faultrecovery suites run while
// they take parts of the network down. The Fabric and Fabric-x suites share it.
package views

import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Namespace is the namespace the suites deploy and write to. Its endorsement policy
// requires Org1, the initiator's organization.
const Namespace = "faultrecovery"

// FinalityTimeout bounds how long each party waits for a transaction to become final.
const FinalityTimeout = time.Minute

// Asset is the state each transaction creates. Its key is generated on creation.
type Asset struct {
	Value int
}

// Submit is the input of SubmitView.
type Submit struct {
	// Value is stored in the created Asset.
	Value int
	// Responder is the FSC node that endorses the transaction after the initiator.
	Responder view.Identity
}

// SubmitView creates an Asset, endorses the transaction itself, collects the
// responder's endorsement, orders it and waits until it is final. It returns the
// transaction ID.
type SubmitView struct {
	Submit
}

func (v *SubmitView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	tx.SetNamespace(Namespace)
	if err := tx.AddOutput(&Asset{Value: v.Value}); err != nil {
		return nil, err
	}

	// The initiator signs first: it belongs to Org1, which the namespace policy requires.
	me := tx.FabricNetworkService().IdentityProvider().DefaultIdentity()
	if _, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, me, v.Responder)); err != nil {
		return nil, err
	}
	if _, err := viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout)); err != nil {
		return nil, err
	}
	return tx.ID(), nil
}

type SubmitViewFactory struct{}

func (*SubmitViewFactory) NewView(in []byte) (view.View, error) {
	f := &SubmitView{}
	if err := json.Unmarshal(in, &f.Submit); err != nil {
		return nil, err
	}
	return f, nil
}

// ResponderView endorses the transaction SubmitView sends and waits until it is final.
type ResponderView struct{}

func (*ResponderView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	if _, err := viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, err
	}
	return viewCtx.RunView(state.NewFinalityWithTimeoutView(tx, FinalityTimeout))
}
