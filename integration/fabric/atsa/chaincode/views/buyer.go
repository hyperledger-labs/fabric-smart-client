/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type AgreeToBuyView struct {
	*AssetPrice
}

func (a *AgreeToBuyView) Call(context view.Context) (interface{}, error) {
	assetPrice, err := json.Marshal(a.AssetPrice)
	assert.NoError(err, "failed marshalling assetPrice")

	_, err = context.RunView(
		chaincode.NewInvokeView(
			"asset_transfer",
			"AgreeToBuy",
			a.AssetID,
		).WithTransientEntry("asset_price", assetPrice).WithEndorsersFromMyOrg(),
	)
	assert.NoError(err, "failed agreeing to sell")
	return nil, nil
}

type AgreeToBuyViewFactory struct{}

func (p *AgreeToBuyViewFactory) NewView(in []byte) (view.View, error) {
	f := &AgreeToBuyView{AssetPrice: &AssetPrice{}}
	err := json.Unmarshal(in, f.AssetPrice)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
