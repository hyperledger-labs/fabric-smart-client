/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type AgreeToSellView struct {
	*AssetPrice
}

func (a *AgreeToSellView) Call(context view.Context) (interface{}, error) {
	assetPrice, err := a.AssetPrice.Bytes()
	assert.NoError(err, "failed marshalling assetPrice")

	_, err = context.RunView(
		chaincode.NewInvokeView(
			"asset_transfer",
			"AgreeToSell",
			a.AssetID,
		).WithTransientEntry("asset_price", assetPrice).WithEndorsersFromMyOrg(),
	)
	assert.NoError(err, "failed agreeing to sell")
	return nil, nil
}

type AgreeToSellViewFactory struct{}

func (p *AgreeToSellViewFactory) NewView(in []byte) (view.View, error) {
	f := &AgreeToSellView{AssetPrice: &AssetPrice{}}
	err := json.Unmarshal(in, f.AssetPrice)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}

type Transfer struct {
	Recipient       view.Identity
	AssetProperties *AssetProperties
	AssetPrice      *AssetPrice
}

type TransferView struct {
	*Transfer
}

func (a *TransferView) Call(context view.Context) (interface{}, error) {
	mspID, err := fabric.GetDefaultChannel(context).MSPManager().GetMSPIdentifier(
		fabric.GetDefaultNetwork(context).IdentityProvider().Identity(a.Recipient.UniqueID()),
	)
	assert.NoError(err, "failed deserializing identity")

	assetPrice, err := a.AssetPrice.Bytes()
	assert.NoError(err, "failed marshalling assetPrice")

	assetProperties, err := a.AssetProperties.Bytes()
	assert.NoError(err, "failed marshalling assetProperties")

	_, err = context.RunView(
		chaincode.NewInvokeView(
			"asset_transfer",
			"TransferAsset",
			a.AssetProperties.ID,
			mspID,
		).WithTransientEntry(
			"asset_price", assetPrice,
		).WithTransientEntry(
			"asset_properties", assetProperties,
		),
	)
	assert.NoError(err, "failed agreeing to sell")
	return nil, nil
}

type TransferViewFactory struct{}

func (p *TransferViewFactory) NewView(in []byte) (view.View, error) {
	f := &TransferView{Transfer: &Transfer{}}
	err := json.Unmarshal(in, f.Transfer)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
