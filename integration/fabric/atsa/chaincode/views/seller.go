/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"encoding/json"
	"time"

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

func (a *TransferView) Call(ctx view.Context) (interface{}, error) {
	senderMSPID, err := fabric.GetDefaultChannel(ctx).MSPManager().GetMSPIdentifier(
		fabric.GetDefaultFNS(ctx).IdentityProvider().DefaultIdentity(),
	)
	assert.NoError(err, "failed getting sender MSP-ID")
	recipientMSPID, err := fabric.GetDefaultChannel(ctx).MSPManager().GetMSPIdentifier(
		fabric.GetDefaultFNS(ctx).IdentityProvider().Identity(a.Recipient.UniqueID()),
	)
	assert.NoError(err, "failed getting recipient MSP-ID")

	assetPrice, err := a.AssetPrice.Bytes()
	assert.NoError(err, "failed marshalling assetPrice")

	assetProperties, err := a.AssetProperties.Bytes()
	assert.NoError(err, "failed marshalling assetProperties")

	envelope, err := fabric.GetDefaultChannel(ctx).Chaincode("asset_transfer").Endorse(
		"TransferAsset",
		a.AssetProperties.ID,
		recipientMSPID,
	).WithTransientEntry(
		"asset_price", assetPrice,
	).WithTransientEntry(
		"asset_properties", assetProperties,
	).WithImplicitCollections(senderMSPID, recipientMSPID).Call()
	assert.NoError(err, "failed asking endorsement")

	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	assert.Error(fabric.GetDefaultChannel(ctx).Delivery().Scan(
		c,
		envelope.TxID(),
		func(tx *fabric.ProcessedTransaction) (bool, error) {
			return tx.TxID() == envelope.TxID(), nil
		},
	), "the transaction [%s] has not been broadcast yet", envelope.TxID())

	assert.NoError(fabric.GetDefaultFNS(ctx).Ordering().Broadcast(ctx.Context(), envelope), "failed sending to ordering")

	c, cancel = context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	assert.NoError(fabric.GetDefaultChannel(ctx).Delivery().Scan(
		c,
		envelope.TxID(),
		func(tx *fabric.ProcessedTransaction) (bool, error) {
			return tx.TxID() == envelope.TxID(), nil
		},
	), "failed agreeing to sell")

	return nil, nil
}

type TransferViewFactory struct{}

func (p *TransferViewFactory) NewView(in []byte) (view.View, error) {
	f := &TransferView{Transfer: &Transfer{}}
	err := json.Unmarshal(in, f.Transfer)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
