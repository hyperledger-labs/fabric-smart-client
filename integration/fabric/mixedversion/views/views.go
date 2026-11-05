/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package views holds the views of the mixed-version suite. The suite runs some
// nodes on the previous FSC release, built from that release's copy of this
// package, so these views must keep building and behaving the same at the
// previous release: use only FSC API that release has, and change a view's
// input or behaviour only together with a new view name.
package views

import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Namespace is the namespace the suite writes to. Its endorsement policy requires
// Org1, the approver's organization.
const Namespace = "mixedversion"

// finalityTimeout bounds how long a party waits for a transaction to become final.
const finalityTimeout = time.Minute

// Asset is the state the suite creates and updates.
type Asset struct {
	ID    string
	Value int
}

// GetLinearID returns the key the Asset is stored under.
func (a *Asset) GetLinearID() (string, error) { return a.ID, nil }

// Create is the input of CreateView.
type Create struct {
	// ID is the key of the new Asset.
	ID    string
	Value int
	// Contract, if set, is stored in the Asset's state metadata (state.WithContract), so
	// the vault stores metadata for it and a later update reads that metadata back.
	Contract string
	// Approver endorses the transaction after the initiator.
	Approver view.Identity
}

// CreateView creates an Asset, collects the initiator's and the approver's
// endorsements, orders the transaction and waits until it is final.
type CreateView struct {
	Create
}

func (v *CreateView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	tx.SetNamespace(Namespace)
	var opts []state.AddOutputOption
	if v.Contract != "" {
		opts = append(opts, state.WithContract(v.Contract))
	}
	if err := tx.AddOutput(&Asset{ID: v.ID, Value: v.Value}, opts...); err != nil {
		return nil, err
	}
	return submit(viewCtx, tx, v.Approver)
}

// Update is the input of UpdateView.
type Update struct {
	ID    string
	Value int
	// Contract, if set, is stored in the state metadata of the updated Asset, as for
	// Create. Without it the update stores no metadata.
	Contract string
	Approver view.Identity
}

// UpdateView spends the Asset ID and writes it back with Value. Spending reads the
// Asset's state metadata from the vault.
type UpdateView struct {
	Update
}

func (v *UpdateView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	tx.SetNamespace(Namespace)
	asset := &Asset{}
	if err := tx.AddInputByLinearID(v.ID, asset); err != nil {
		return nil, err
	}
	asset.Value = v.Value
	var opts []state.AddOutputOption
	if v.Contract != "" {
		opts = append(opts, state.WithContract(v.Contract))
	}
	if err := tx.AddOutput(asset, opts...); err != nil {
		return nil, err
	}
	return submit(viewCtx, tx, v.Approver)
}

// submit collects the endorsements of the initiator, which signs with its default
// identity, and of approver, then orders tx and waits until it is final. It returns
// the transaction ID.
func submit(viewCtx view.Context, tx *state.Transaction, approver view.Identity) (any, error) {
	me := tx.FabricNetworkService().IdentityProvider().DefaultIdentity()
	if _, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, me, approver)); err != nil {
		return nil, err
	}
	if _, err := viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, finalityTimeout)); err != nil {
		return nil, err
	}
	return tx.ID(), nil
}

// ApproverView endorses the transactions of CreateView and UpdateView and waits
// until they are final.
type ApproverView struct{}

func (*ApproverView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	if _, err := viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, err
	}
	return viewCtx.RunView(state.NewFinalityWithTimeoutView(tx, finalityTimeout))
}

// Query is the input of QueryView.
type Query struct {
	ID string
}

// QueryView returns the value of the Asset ID in the node's vault.
type QueryView struct {
	Query
}

func (v *QueryView) Call(viewCtx view.Context) (any, error) {
	_, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, err
	}
	qe, err := ch.Vault().NewQueryExecutor(viewCtx.Context())
	if err != nil {
		return nil, err
	}
	defer func() { _ = qe.Done() }()
	read, err := qe.GetState(viewCtx.Context(), Namespace, v.ID)
	if err != nil {
		return nil, err
	}
	if read == nil || len(read.Raw) == 0 {
		return nil, errors.Errorf("asset [%s] not found", v.ID)
	}
	asset := &Asset{}
	if err := json.Unmarshal(read.Raw, asset); err != nil {
		return nil, err
	}
	return asset.Value, nil
}

type CreateViewFactory struct{}

func (*CreateViewFactory) NewView(in []byte) (view.View, error) {
	f := &CreateView{}
	return f, json.Unmarshal(in, &f.Create)
}

type UpdateViewFactory struct{}

func (*UpdateViewFactory) NewView(in []byte) (view.View, error) {
	f := &UpdateView{}
	return f, json.Unmarshal(in, &f.Update)
}

type QueryViewFactory struct{}

func (*QueryViewFactory) NewView(in []byte) (view.View, error) {
	f := &QueryView{}
	return f, json.Unmarshal(in, &f.Query)
}
