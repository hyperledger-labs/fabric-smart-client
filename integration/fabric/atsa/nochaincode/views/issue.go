/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Issue struct {
	Asset    *Asset
	Approver view.Identity
}

type IssueView struct {
	*Issue
}

func (f *IssueView) Call(context view.Context) (interface{}, error) {
	// Prepare transaction
	tx, err := state.NewTransaction(context)
	assert.NoError(err, "failed creating transaction")
	tx.SetNamespace("asset_transfer")
	me := fabric.GetIdentityProvider(context).DefaultIdentity()
	assert.NoError(tx.AddCommand("issue", me, f.Asset.Owner), "failed adding issue command")
	assert.NoError(tx.AddOutput(f.Asset), "failed adding output")

	_, err = context.RunView(state.NewCollectEndorsementsView(tx, me, f.Asset.Owner))
	assert.NoError(err, "failed collecting endorsement")

	_, err = context.RunView(state.NewCollectApprovesView(tx, f.Approver))
	assert.NoError(err, "failed collecting approves")

	// Send to the ordering service and wait for confirmation
	_, err = context.RunView(state.NewOrderingView(tx))
	assert.NoError(err, "failed asking ordering")

	return tx.ID(), nil
}

type IssueViewFactory struct{}

func (p *IssueViewFactory) NewView(in []byte) (view.View, error) {
	f := &IssueView{Issue: &Issue{}}
	err := json.Unmarshal(in, f.Issue)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
