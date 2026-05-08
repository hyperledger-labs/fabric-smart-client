/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Namespace is the Fabric-X namespace this example writes to. It corresponds
// to the channel-or-collection-or-chaincode-id concept of classical Fabric;
// Fabric-X partitions a single channel into namespaces, each governed by its
// own endorsement policy.
const Namespace = "asset-transfer"

// FinalityTimeout caps how long a view waits for the Fabric-X-Committer to
// report a transaction as committed before giving up.
const FinalityTimeout = 1 * time.Minute

// InitParams carries the input to the InitLedgerView.
//
// Endorser and Auditor are the FSC identities of the endorser and auditor
// nodes. The initiator collects their signatures during CollectEndorsements.
type InitParams struct {
	Endorser view.Identity
	Auditor  view.Identity
}

// InitLedgerView is the FSC analogue of the InitLedger chaincode method.
//
// Classical chaincode (asset-transfer-basic):
//
//	for _, asset := range assets { ctx.GetStub().PutState(asset.ID, json) }
//
// FSC view:
//
//	build one transaction with six AddOutput calls; CollectEndorsements
//	from issuer + endorser + auditor; submit to orderer; wait for finality.
//
// The six assets and their values match the chaincode constants verbatim.
type InitLedgerView struct {
	InitParams
}

// SeedAssets returns the six bootstrap assets as defined by the chaincode.
// Exposed as a function rather than a global so tests can match against it.
func SeedAssets() []*states.Asset {
	return []*states.Asset{
		{ID: "asset1", Color: "blue", Size: 5, Owner: "Tomoko", AppraisedValue: 300},
		{ID: "asset2", Color: "red", Size: 5, Owner: "Brad", AppraisedValue: 400},
		{ID: "asset3", Color: "green", Size: 10, Owner: "Jin Soo", AppraisedValue: 500},
		{ID: "asset4", Color: "yellow", Size: 10, Owner: "Max", AppraisedValue: 600},
		{ID: "asset5", Color: "black", Size: 15, Owner: "Adriana", AppraisedValue: 700},
		{ID: "asset6", Color: "white", Size: 15, Owner: "Michel", AppraisedValue: 800},
	}
}

func (i *InitLedgerView) Call(viewCtx view.Context) (interface{}, error) {
	tx, err := state.NewTransaction(viewCtx)
	assert.NoError(err, "InitLedger failed creating transaction")
	tx.SetNamespace(Namespace)
	assert.NoError(tx.AddCommand("init"), "InitLedger failed adding command")

	for _, asset := range SeedAssets() {
		assert.NoError(tx.AddOutput(asset), "InitLedger failed adding asset %s", asset.ID)
	}

	// CollectEndorsements gathers signatures in order: initiator (issuer),
	// then the endorser (chaincode replacement), then the auditor.
	_, err = viewCtx.RunView(state.NewCollectEndorsementsView(tx, i.Endorser, i.Auditor))
	assert.NoError(err, "InitLedger failed collecting endorsements")

	// Register a finality listener BEFORE submitting so we don't miss the
	// commit notification on a fast-finishing transaction.
	var wg sync.WaitGroup
	wg.Add(1)
	_, ch, err := fabric.GetDefaultChannel(viewCtx)
	assert.NoError(err)
	committer := ch.Committer()
	assert.NoError(committer.AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), fdriver.Valid, &wg)),
		"InitLedger failed registering finality listener")

	_, err = viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout))
	assert.NoError(err, "InitLedger failed ordering")
	wg.Wait()

	return tx.ID(), nil
}

type InitLedgerViewFactory struct{}

func (*InitLedgerViewFactory) NewView(in []byte) (view.View, error) {
	f := &InitLedgerView{}
	if len(in) > 0 {
		assert.NoError(json.Unmarshal(in, &f.InitParams), "InitLedger bad input")
	}
	return f, nil
}

// EndorserInitView prepares the endorser FSC node to process the namespace.
// In classical Fabric the chaincode lifecycle (approve, commit) plays this
// role; here the namespace approval happened as part of NWO bootstrap and
// we just tell the committer that we want to process its events.
type EndorserInitView struct{}

func (e *EndorserInitView) Call(viewCtx view.Context) (interface{}, error) {
	_, ch, err := fabric.GetDefaultChannel(viewCtx)
	assert.NoError(err)
	assert.NoError(ch.Committer().ProcessNamespace(Namespace), "endorser init: process namespace")
	return nil, nil
}

type EndorserInitViewFactory struct{}

func (*EndorserInitViewFactory) NewView(in []byte) (view.View, error) {
	return &EndorserInitView{}, nil
}
