/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Issue struct {
	// Asset to be issued
	Asset *states.Asset
	// Recipient is the identity of the recipient's FSC node
	Recipient view.Identity
	// Approver is the identity of the approver's FSC node
	Approver view.Identity
}

type IssueView struct {
	*Issue
}

func (f *IssueView) Call(context view.Context) (interface{}, error) {
	// As a first step operation, the issuer contacts the recipient's FSC node
	// to request the identity to use to assign ownership of the freshly created asset.
	assetOwner, err := state.RequestRecipientIdentity(context, f.Recipient)
	assert.NoError(err, "failed getting recipient identity")

	// The issuer creates a new transaction
	tx, err := state.NewTransaction(context)
	assert.NoError(err, "failed creating transaction")

	// Sets the namespace where the state should be stored
	tx.SetNamespace("asset_transfer")

	f.Asset.Owner = assetOwner
	me := fabric.GetDefaultIdentityProvider(context).DefaultIdentity()

	// Specifies the command this transaction wants to execute.
	// In particular, the issuer wants to create a new asset owned by a given recipient.
	// The approver will use this information to decide how validate the transaction
	assert.NoError(tx.AddCommand("issue", me, f.Asset.Owner), "failed adding issue command")

	// The issuer adds the asset to the transaction
	assert.NoError(tx.AddOutput(f.Asset), "failed adding output")

	// The issuer is ready to collect all the required signatures.
	// Namely from the issuer itself, the lender, and the approver. In this order.
	// All signatures are required.
	_, err = context.RunView(state.NewCollectEndorsementsView(tx, me, f.Asset.Owner, f.Approver))
	assert.NoError(err, "failed collecting endorsement")

	// At this point the issuer can send the transaction to the ordering service and wait for finality.
	_, err = context.RunView(state.NewOrderingAndFinalityView(tx))
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
