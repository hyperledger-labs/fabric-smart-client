/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// UpdateParams is the input to UpdateAssetView. ID identifies the asset to
// update; the remaining four fields overwrite the corresponding chaincode
// fields. The migration deliberately keeps the chaincode signature.
type UpdateParams struct {
	ID             string
	Color          string
	Size           int
	Owner          string
	AppraisedValue int

	Endorser view.Identity
	Auditor  view.Identity
}

// UpdateAssetView is the FSC analogue of:
//
//	func (s *SmartContract) UpdateAsset(ctx, id, color, size, owner, value) error
//
// The chaincode fetched the asset to confirm existence then PutState with a
// fresh struct (overwriting). The view here uses tx.AddInputByLinearID to
// load the existing asset (which serves both as the existence check and as
// the input to the read-set), then constructs the new value as the output.
type UpdateAssetView struct {
	UpdateParams
}

func (u *UpdateAssetView) Call(viewCtx view.Context) (interface{}, error) {
	tx, err := state.NewTransaction(viewCtx)
	assert.NoError(err)
	tx.SetNamespace(Namespace)
	assert.NoError(tx.AddCommand("update"))

	// Load the existing asset as the transaction's input (read-set entry).
	// If the asset does not exist this errors out before we touch the orderer.
	in := &states.Asset{}
	assert.NoError(tx.AddInputByLinearID(u.ID, in), "Update: load existing asset %s", u.ID)

	out := &states.Asset{
		ID:             u.ID,
		Color:          u.Color,
		Size:           u.Size,
		Owner:          u.Owner,
		AppraisedValue: u.AppraisedValue,
	}
	assert.NoError(tx.AddOutput(out))

	_, err = viewCtx.RunView(state.NewCollectEndorsementsView(tx, u.Endorser, u.Auditor))
	assert.NoError(err)

	var wg sync.WaitGroup
	wg.Add(1)
	_, ch, err := fabric.GetDefaultChannel(viewCtx)
	assert.NoError(err)
	committer := ch.Committer()
	assert.NoError(committer.AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), fdriver.Valid, &wg)))

	_, err = viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout))
	assert.NoError(err)
	wg.Wait()

	return tx.ID(), nil
}

type UpdateAssetViewFactory struct{}

func (*UpdateAssetViewFactory) NewView(in []byte) (view.View, error) {
	f := &UpdateAssetView{}
	assert.NoError(json.Unmarshal(in, &f.UpdateParams), "Update bad input")
	return f, nil
}
