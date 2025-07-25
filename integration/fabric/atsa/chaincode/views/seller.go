/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type AgreeToSellView struct {
	*AssetPrice
}

func (a *AgreeToSellView) Call(context view.Context) (interface{}, error) {
	assetPrice, err := a.Bytes()
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
	_, ch, err := fabric.GetDefaultChannel(ctx)
	assert.NoError(err)
	fns, err := fabric.GetDefaultFNS(ctx)
	assert.NoError(err)

	senderMSPID, err := ch.MSPManager().GetMSPIdentifier(
		fns.IdentityProvider().DefaultIdentity(),
	)
	assert.NoError(err, "failed getting sender MSP-ID")
	recipientMSPID, err := ch.MSPManager().GetMSPIdentifier(
		assert.ValidIdentity(fns.IdentityProvider().Identity(a.Recipient.UniqueID())),
	)
	assert.NoError(err, "failed getting recipient MSP-ID")

	assetPrice, err := a.AssetPrice.Bytes()
	assert.NoError(err, "failed marshalling assetPrice")

	assetProperties, err := a.AssetProperties.Bytes()
	assert.NoError(err, "failed marshalling assetProperties")

	endorse, err := ch.Chaincode("asset_transfer").Endorse(
		"TransferAsset",
		a.AssetProperties.ID,
		recipientMSPID,
	).WithImplicitCollections(
		senderMSPID, recipientMSPID,
	).WithTransientEntries(map[string]interface{}{
		"asset_price":      assetPrice,
		"asset_properties": assetProperties,
	})
	assert.NoError(err, "failed agreeing to sell")
	envelope, err := endorse.Call()
	assert.NoError(err, "failed asking endorsement")

	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	assert.Error(ch.Delivery().Scan(
		c,
		envelope.TxID(),
		func(tx *fabric.ProcessedTransaction) (bool, error) {
			return tx.TxID() == envelope.TxID(), nil
		},
	), "the transaction [%s] has not been broadcast yet", envelope.TxID())

	assert.NoError(fns.Ordering().Broadcast(ctx.Context(), envelope), "failed sending to ordering")

	c, cancel = context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	assert.NoError(ch.Delivery().Scan(
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
