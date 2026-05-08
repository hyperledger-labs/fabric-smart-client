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

// CreateParams carries the inputs to CreateAssetView. The five business
// fields mirror the chaincode signature exactly:
//
//	CreateAsset(ctx, id, color, size, owner, appraisedValue)
//
// The Endorser and Auditor identities are FSC-specific — they tell the
// initiator which FSC nodes to collect endorsement signatures from. In the
// chaincode world the endorsement targets were resolved from the chaincode
// definition and the channel; here they are explicit.
type CreateParams struct {
	ID             string
	Color          string
	Size           int
	Owner          string
	AppraisedValue int

	Endorser view.Identity
	Auditor  view.Identity
}

// CreateAssetView is the FSC analogue of:
//
//	func (s *SmartContract) CreateAsset(ctx, id, color, size, owner, value) error
//
// The chaincode body checked AssetExists and then PutState. The view here
// builds a transaction with one AddOutput, lets the endorser do the
// existence check on its own (via the Query Service), collects signatures,
// and submits.
type CreateAssetView struct {
	CreateParams
}

func (c *CreateAssetView) Call(viewCtx view.Context) (interface{}, error) {
	tx, err := state.NewTransaction(viewCtx)
	assert.NoError(err, "Create failed creating tx")
	tx.SetNamespace(Namespace)
	assert.NoError(tx.AddCommand("create"), "Create failed adding command")

	asset := &states.Asset{
		ID:             c.ID,
		Color:          c.Color,
		Size:           c.Size,
		Owner:          c.Owner,
		AppraisedValue: c.AppraisedValue,
	}
	assert.NoError(tx.AddOutput(asset), "Create failed adding output")

	_, err = viewCtx.RunView(state.NewCollectEndorsementsView(tx, c.Endorser, c.Auditor))
	assert.NoError(err, "Create failed collecting endorsements")

	var wg sync.WaitGroup
	wg.Add(1)
	_, ch, err := fabric.GetDefaultChannel(viewCtx)
	assert.NoError(err)
	committer := ch.Committer()
	assert.NoError(committer.AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), fdriver.Valid, &wg)))

	_, err = viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout))
	assert.NoError(err, "Create failed ordering")
	wg.Wait()

	return tx.ID(), nil
}

type CreateAssetViewFactory struct{}

func (*CreateAssetViewFactory) NewView(in []byte) (view.View, error) {
	f := &CreateAssetView{}
	assert.NoError(json.Unmarshal(in, &f.CreateParams), "Create bad input")
	return f, nil
}
