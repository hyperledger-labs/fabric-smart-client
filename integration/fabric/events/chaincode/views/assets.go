/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type CreateAsset struct {
	Asset Asset
}

type CreateAssetView struct {
	*CreateAsset
}

var logger = flogging.MustGetLogger("assets")

func (c *CreateAssetView) Call(context view.Context) (interface{}, error) {
	//Register chaincodeEvents
	callBack := func(event *committer.ChaincodeEvent) error {
		logger.Debugf("Event Received in callback ", event)
		return nil
	}
	_, err := context.RunView(chaincode.NewRegisterChaincodeView("asset_transfer_events", callBack))
	assert.NoError(err, "failed registering to events")

	if err != nil {
		return nil, err
	}

	_, err1 := context.RunView(
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
	if err1 != nil {
		return nil, err
	}
	return nil, nil

}

type CreateAssetViewFactory struct{}

func (c *CreateAssetViewFactory) NewView(in []byte) (view.View, error) {
	f := &CreateAssetView{CreateAsset: &CreateAsset{}}
	err := json.Unmarshal(in, f.CreateAsset)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
