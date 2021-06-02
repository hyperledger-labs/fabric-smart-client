/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Query struct {
	LinearID string
}

type QueryView struct {
	Query
}

func (q *QueryView) Call(context view.Context) (interface{}, error) {
	iouState := &IOUState{}
	err := state.GetWorldState(context).GetState("iou", q.LinearID, iouState)
	assert.NoError(err)
	return iouState.Amount, nil
}
