/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type CreateAsset struct {
	Asset Asset
}

type CreateAssetView struct {
	*CreateAsset
}

func (c *CreateAssetView) Call(context view.Context) (interface{}, error) {

	//Register chaincodeEvents
	events, err := context.RunView(
		chaincode.NewRegisterChaincodeView(
			"asset_transfer_events"))
	assert.NoError(err, "failed registering to events")
	go func() {
		for event := range events.(<-chan *committer.ChaincodeEvent) {
			fmt.Println("event received", event)
		}
	}()

	_, err = context.RunView(
		chaincode.NewInvokeView(
			"asset_transfer_events",
			"CreateAsset",
			c.Asset.ID,
			c.Asset.Color,
			c.Asset.Size,
			c.Asset.Owner,
			c.Asset.AppraisedValue,
		),
	)
	assert.NoError(err, "failed creating asset")
	return nil, nil
}

type CreateAssetViewFactory struct{}

func (c *CreateAssetViewFactory) NewView(in []byte) (view.View, error) {
	f := &CreateAssetView{CreateAsset: &CreateAsset{}}
	err := json.Unmarshal(in, f.CreateAsset)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
