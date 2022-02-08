/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger-labs/fabric-smart-client/samples/fabric/iou/states"
)

type Query struct {
	LinearID string
}

type QueryView struct {
	Query
}

func (q *QueryView) Call(context view.Context) (interface{}, error) {
	iouState := &states.IOU{}
	err := state.GetVault(context).GetState("iou", q.LinearID, iouState)
	assert.NoError(err)
	return iouState.Amount, nil
}

type QueryViewFactory struct{}

func (c *QueryViewFactory) NewView(in []byte) (view.View, error) {
	f := &QueryView{}
	err := json.Unmarshal(in, &f.Query)
	assert.NoError(err)
	return f, nil
}
