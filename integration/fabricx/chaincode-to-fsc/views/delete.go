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

// DeleteParams identifies the asset to delete plus the FSC identities
// needed for endorsement.
type DeleteParams struct {
	ID       string
	Endorser view.Identity
	Auditor  view.Identity
}

// DeleteAssetView is the FSC analogue of:
//
//	func (s *SmartContract) DeleteAsset(ctx, id) error
//
// Pattern: load the existing asset as input, produce no output. The state
// package commits this as a deletion of the world-state key on the
// committer side; the endorser's command="delete" check enforces the shape.
type DeleteAssetView struct {
	DeleteParams
}

func (d *DeleteAssetView) Call(viewCtx view.Context) (interface{}, error) {
	tx, err := state.NewTransaction(viewCtx)
	assert.NoError(err)
	tx.SetNamespace(Namespace)
	assert.NoError(tx.AddCommand("delete"))

	in := &states.Asset{}
	assert.NoError(tx.AddInputByLinearID(d.ID, in), "Delete: load existing asset %s", d.ID)
	// No AddOutput — the absence of an output keyed at d.ID is what causes
	// the world-state key to be removed at commit time.

	_, err = viewCtx.RunView(state.NewCollectEndorsementsView(tx, d.Endorser, d.Auditor))
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

type DeleteAssetViewFactory struct{}

func (*DeleteAssetViewFactory) NewView(in []byte) (view.View, error) {
	f := &DeleteAssetView{}
	assert.NoError(json.Unmarshal(in, &f.DeleteParams), "Delete bad input")
	return f, nil
}
