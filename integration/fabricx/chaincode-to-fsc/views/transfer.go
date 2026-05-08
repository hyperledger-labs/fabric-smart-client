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

// TransferParams is the input to TransferAssetView. NewOwnerLabel is the
// chaincode-style string the asset's Owner field will be set to. NewOwner
// is the FSC identity of the receiver — the receiver's responder view will
// be invoked at this identity.
type TransferParams struct {
	ID            string
	NewOwnerLabel string

	NewOwner view.Identity
	Endorser view.Identity
	Auditor  view.Identity
}

// TransferAssetView is the FSC analogue of:
//
//	func (s *SmartContract) TransferAsset(ctx, id, newOwner) (string, error)
//
// The chaincode read the asset, mutated Owner, and PutState. There was no
// way for the new owner to refuse the asset — anyone with rights to invoke
// the chaincode could write any Owner string.
//
// In FSC-on-Fabric-X we make the receiver an explicit participant: the new
// owner's FSC node runs TransferAssetReceiverView (registered as a responder
// in the topology) and signs the transaction only if it accepts. This is a
// strict tightening of the chaincode security model.
type TransferAssetView struct {
	TransferParams
}

func (t *TransferAssetView) Call(viewCtx view.Context) (interface{}, error) {
	tx, err := state.NewTransaction(viewCtx)
	assert.NoError(err)
	tx.SetNamespace(Namespace)
	assert.NoError(tx.AddCommand("transfer"))

	// Read the existing asset (existence check + read-set).
	in := &states.Asset{}
	assert.NoError(tx.AddInputByLinearID(t.ID, in), "Transfer: load existing asset %s", t.ID)

	// Sanity-guard the trivial no-op transfer at the initiator side. The
	// endorser also enforces this, but failing fast here saves a round trip.
	assert.NotEqual(in.Owner, t.NewOwnerLabel, "Transfer: asset %s already owned by %s", t.ID, t.NewOwnerLabel)

	// Build the output — same shape as the input, only Owner changes.
	out := &states.Asset{
		ID:             in.ID,
		Color:          in.Color,
		Size:           in.Size,
		Owner:          t.NewOwnerLabel,
		AppraisedValue: in.AppraisedValue,
	}
	assert.NoError(tx.AddOutput(out))

	// Collect endorsements: receiver first, so the receiver's refusal
	// short-circuits before we bother the endorser; then the endorser
	// (chaincode logic); then the auditor.
	_, err = viewCtx.RunView(state.NewCollectEndorsementsView(tx, t.NewOwner, t.Endorser, t.Auditor))
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

	// Mirror the chaincode's return value (oldOwner) so client code doesn't
	// have to change its post-transfer accounting.
	return in.Owner, nil
}

type TransferAssetViewFactory struct{}

func (*TransferAssetViewFactory) NewView(in []byte) (view.View, error) {
	f := &TransferAssetView{}
	assert.NoError(json.Unmarshal(in, &f.TransferParams), "Transfer bad input")
	return f, nil
}

// TransferAssetReceiverView is the receiver-acceptance responder. The new
// owner's FSC node runs this when the initiator sends the assembled
// transaction during CollectEndorsements. The receiver inspects the
// transaction, decides whether to accept, and either signs or returns an
// error.
//
// This view is the migration's most architecturally interesting moment.
// Classical chaincode could not express receiver acceptance because there
// is no per-key endorser identity at chaincode level. FSC views give us
// that identity for free.
type TransferAssetReceiverView struct{}

func (r *TransferAssetReceiverView) Call(viewCtx view.Context) (interface{}, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	assert.NoError(err, "Receiver: receive tx")

	assert.Equal(1, tx.Commands().Count(), "Receiver: expected one command")
	assert.Equal("transfer", tx.Commands().At(0).Name, "Receiver: expected transfer command")

	// Read the proposed new state and decide whether we accept it. Real
	// production code can hook business rules here — credit checks, AML,
	// anti-self-dealing, etc. For this tutorial we simply assert that the
	// asset has a non-empty new owner string.
	out := &states.Asset{}
	assert.NoError(tx.GetOutputAt(0, out))
	assert.NotEmpty(out.Owner, "Receiver: refusing transfer with empty Owner")

	// Sign and send the transaction back to the initiator.
	_, err = viewCtx.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Wait for finality on the receiver side too. This way the receiver
	// knows when the transfer has been committed and can update local
	// caches / send notifications.
	return viewCtx.RunView(state.NewFinalityWithTimeoutView(tx, 1*time.Minute))
}
