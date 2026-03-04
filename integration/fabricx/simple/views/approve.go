/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ApproveView struct{}

func (i *ApproveView) Call(viewCtx view.Context) (interface{}, error) {
	logger.Infof("Approve View called! great")

	network, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, err
	}
	qs, err := queryservice.GetQueryService(viewCtx, network.Name(), ch.Name())
	if err != nil {
		return nil, err
	}

	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, err
	}

	// check that tx has a create command
	if tx.Commands().Count() != 1 {
		return nil, fmt.Errorf("cmd count is wrong, explected 1 but got %d", tx.Commands().Count())
	}

	cmd := tx.Commands().At(0)
	if cmd.Name != "create" {
		return nil, fmt.Errorf("cmd type is wrong, explected `create` but got %s", cmd.Name)
	}

	if tx.NumOutputs() != 1 {
		return nil, fmt.Errorf("num of output must be 1, got %d", tx.NumOutputs())
	}

	obj := &SomeObject{}
	if err = tx.GetOutputAt(0, obj); err != nil {
		return nil, err
	}

	logger.Infof("Check with the committer if the object exists ( using the QS )")
	objKey, err := obj.GetLinearID()
	if err != nil {
		return nil, err
	}

	val, err := qs.GetState("simple", objKey)
	if err != nil {
		return nil, err
	}

	// note that this obj should not yet exist
	if val != nil {
		return nil, fmt.Errorf("this should be an error")
	}

	// some additional checks
	if obj.Value < 0 {
		return nil, fmt.Errorf("obj value can not be smaller than 0, go %d", obj.Value)
	}

	// The approver is ready to send back the transaction signed
	if _, err = viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, err
	}

	logger.Infof("Finished approving")

	return nil, nil
}
