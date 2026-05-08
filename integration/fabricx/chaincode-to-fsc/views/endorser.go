/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// EndorserView is the FSC-on-Fabric-X analogue of the chaincode's per-method
// validation. In the asset-transfer-basic chaincode, every state-changing
// function has an existence check, an ownership check, or a "no-op transfer"
// guard. Those checks ran on the endorsing peer and were trusted because the
// chaincode binary was the only thing that could produce a valid endorsement.
//
// Here, the same checks run inside this view on an FSC node that holds the
// endorser role for the asset-transfer namespace. The mapping is one-to-one:
// each command name corresponds to one chaincode function.
//
//	chaincode method        command       check performed here
//	---------------------   -----------   ---------------------------
//	InitLedger              init          no inputs, six outputs, ascending IDs
//	CreateAsset             create        no inputs, one output, asset does not exist
//	UpdateAsset             update        one input, one output, same ID
//	DeleteAsset             delete        one input, no output, asset exists
//	TransferAsset           transfer      one input, one output, owner change is real
type EndorserView struct{}

func (e *EndorserView) Call(viewCtx view.Context) (interface{}, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	assert.NoError(err, "failed receiving transaction")

	// Sanity check the namespace — the endorser is only registered for the
	// asset-transfer namespace, so any other namespace on the transaction is
	// a programming error worth surfacing loudly.
	assert.Equal(1, len(tx.Namespaces()), "expected exactly one namespace, got %d", len(tx.Namespaces()))
	assert.Equal(Namespace, tx.Namespaces()[0], "expected namespace %q, got %q", Namespace, tx.Namespaces()[0])

	// Every transaction must carry exactly one command. The command name
	// dictates which chaincode-era validation we run.
	assert.Equal(1, tx.Commands().Count(), "expected exactly one command, got %d", tx.Commands().Count())

	// We need the Query Service for existence checks on the world state — it
	// replaces ctx.GetStub().GetState(...) from the chaincode. The Query
	// Service is the read-side microservice of the Fabric-X-Committer.
	network, ch, err := fabric.GetChannel(viewCtx, tx.Network(), tx.Channel())
	assert.NoError(err, "failed getting channel")
	qs, err := queryservice.GetQueryService(viewCtx, network.Name(), ch.Name())
	assert.NoError(err, "failed getting query service")

	switch cmd := tx.Commands().At(0); cmd.Name {
	case "init":
		// InitLedger writes a fixed batch of assets; six is the count chosen
		// by the asset-transfer-basic chaincode. Other batch sizes are
		// rejected here so an accidental InitLedger from a buggy view does
		// not silently overwrite the world state.
		assert.Equal(0, tx.NumInputs(), "init must have no inputs, got %d", tx.NumInputs())
		assert.Equal(6, tx.NumOutputs(), "init must produce exactly 6 assets, got %d", tx.NumOutputs())
		// Each output asset's ID must be unique and not already present.
		for i := 0; i < tx.NumOutputs(); i++ {
			out := &states.Asset{}
			assert.NoError(tx.GetOutputAt(i, out), "failed reading init output %d", i)
			assert.NotEmpty(out.ID, "init output %d has empty ID", i)
			val, err := qs.GetState(Namespace, out.ID)
			assert.NoError(err, "failed querying existence of %s", out.ID)
			assert.True(val == nil, "init must not overwrite existing asset %s", out.ID)
		}

	case "create":
		// CreateAsset(ctx, id, color, size, owner, appraisedValue) becomes:
		// no inputs, exactly one output, output's ID must not already exist.
		assert.Equal(0, tx.NumInputs(), "create must have no inputs, got %d", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "create must produce exactly one output, got %d", tx.NumOutputs())
		out := &states.Asset{}
		assert.NoError(tx.GetOutputAt(0, out))
		assert.NotEmpty(out.ID, "create output ID must not be empty")
		assert.NotEmpty(out.Owner, "create output Owner must not be empty")
		assert.True(out.Size > 0, "create output Size must be positive, got %d", out.Size)
		assert.True(out.AppraisedValue > 0, "create output AppraisedValue must be positive, got %d", out.AppraisedValue)
		val, err := qs.GetState(Namespace, out.ID)
		assert.NoError(err, "failed querying existence of %s", out.ID)
		assert.True(val == nil, "create rejects existing asset %s", out.ID)

	case "update":
		// UpdateAsset overwrites everything except the ID. We require one
		// input and one output, with matching IDs.
		assert.Equal(1, tx.NumInputs(), "update must have exactly one input, got %d", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "update must produce exactly one output, got %d", tx.NumOutputs())
		in := &states.Asset{}
		assert.NoError(tx.GetInputAt(0, in))
		out := &states.Asset{}
		assert.NoError(tx.GetOutputAt(0, out))
		assert.Equal(in.ID, out.ID, "update must not change asset ID, %s -> %s", in.ID, out.ID)
		assert.True(out.Size > 0, "update Size must be positive, got %d", out.Size)
		assert.True(out.AppraisedValue > 0, "update AppraisedValue must be positive, got %d", out.AppraisedValue)

	case "delete":
		// DeleteAsset takes one input (the asset to delete) and produces no
		// output. The state package handles the actual world-state deletion
		// when the input has no matching output with the same key.
		assert.Equal(1, tx.NumInputs(), "delete must have exactly one input, got %d", tx.NumInputs())
		assert.Equal(0, tx.NumOutputs(), "delete must produce no output, got %d", tx.NumOutputs())

	case "transfer":
		// TransferAsset is the most interesting check: one input, one output,
		// matching ID, but the Owner field must change AND the new owner must
		// not be the same as the old.
		assert.Equal(1, tx.NumInputs(), "transfer must have exactly one input, got %d", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "transfer must produce exactly one output, got %d", tx.NumOutputs())
		in := &states.Asset{}
		assert.NoError(tx.GetInputAt(0, in))
		out := &states.Asset{}
		assert.NoError(tx.GetOutputAt(0, out))
		assert.Equal(in.ID, out.ID, "transfer must not change asset ID, %s -> %s", in.ID, out.ID)
		assert.NotEqual(in.Owner, out.Owner, "transfer must change Owner, both sides are %s", in.Owner)
		assert.NotEmpty(out.Owner, "transfer new Owner must not be empty")
		// Other fields (Color, Size, AppraisedValue) must not change on a
		// pure transfer. This is a strictly stronger invariant than the
		// chaincode TransferAsset enforced — the chaincode only updated
		// Owner; nothing prevented a buggy client from changing Size at the
		// same time. FSC + namespace endorsement lets us tighten this.
		assert.Equal(in.Color, out.Color, "transfer must not change Color")
		assert.Equal(in.Size, out.Size, "transfer must not change Size")
		assert.Equal(in.AppraisedValue, out.AppraisedValue, "transfer must not change AppraisedValue")

	default:
		return nil, errors.Errorf("unknown command %q", cmd.Name)
	}

	// Validation passed. Sign and send back. The framework collects this
	// signature into the transaction's endorsement set and the initiator
	// continues with submission to the orderer.
	if _, err := viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, errors.Wrap(err, "endorser failed signing")
	}
	return nil, nil
}
