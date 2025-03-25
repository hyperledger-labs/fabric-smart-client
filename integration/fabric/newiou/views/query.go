/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Query struct {
	LinearID string
}

type QueryView struct {
	Query

	vaultService state.VaultService
}

func (q *QueryView) Call(context view.Context) (interface{}, error) {
	iouState := &states.IOU{}
	vault, err := q.vaultService.Vault(fabric.DefaultNetwork, fabric.DefaultChannel)
	assert.NoError(err)
	assert.NoError(vault.GetState(context.Context(), "iou", q.LinearID, iouState))
	return iouState.Amount, nil
}

func NewQueryViewFactory(vaultService state.VaultService) *QueryViewFactory {
	return &QueryViewFactory{vaultService: vaultService}
}

type QueryViewFactory struct {
	vaultService state.VaultService
}

func (c *QueryViewFactory) NewView(in []byte) (view.View, error) {
	f := &QueryView{vaultService: c.vaultService}
	err := json.Unmarshal(in, &f.Query)
	assert.NoError(err)
	return f, nil
}
