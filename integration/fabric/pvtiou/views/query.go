/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/states"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/pvtiou/fabric/couchdb"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const QueryByLinearID = `{"selector":{"LinearID":"%s"}}`

type Query struct {
	LinearID string
}

type QueryView struct {
	Query
}

func (q *QueryView) Call(context view.Context) (interface{}, error) {
	// Run the query using couchdb
	manager := couchdb.GetManager(context)
	assert.NotNil(manager, "failed to get couchdb managre")

	db, err := manager.DB(context, "iou")
	assert.NoError(err, "failed to get default db")

	rows, err := db.Find(context.Context(), fmt.Sprintf(QueryByLinearID, q.LinearID))
	assert.NoError(err, "failed to find state")
	assert.True(rows.Next(), "expect to have a doc")
	iouState1 := &states.IOU{}
	assert.NoError(rows.ScanDoc(iouState1), "failed to retrieve iou state")

	iouState := &states.IOU{}
	assert.NoError(state.GetVault(context).GetState("iou", q.LinearID, iouState))

	assert.Equal(iouState, iouState1, "iou states do not match")

	return iouState.Amount, nil
}

type QueryViewFactory struct{}

func (c *QueryViewFactory) NewView(in []byte) (view.View, error) {
	f := &QueryView{}
	err := json.Unmarshal(in, &f.Query)
	assert.NoError(err)
	return f, nil
}
