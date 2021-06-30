/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/chaincode/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ViewClient interface {
	CallView(fid string, in []byte) (interface{}, error)
	IsTxFinal(txid string) error
}

type Client struct {
	id view.Identity
	c  ViewClient
}

func NewClient(c ViewClient, id view.Identity) *Client {
	return &Client{c: c, id: id}
}

func (c *Client) CreateAsset(ap *views.AssetProperties, publicDescription string) error {
	_, err := c.c.CallView("CreateAsset", common.JSONMarshall(&views.CreateAsset{
		AssetProperties:   ap,
		PublicDescription: publicDescription,
	}))
	return err
}

func (c *Client) ReadAssetPrivateProperties(id string) (*views.AssetProperties, error) {
	result, err := c.c.CallView("ReadAssetPrivateProperties", common.JSONMarshall(
		&views.ReadAssetPrivateProperties{
			ID: id,
		},
	))
	if err != nil {
		return nil, err
	}
	ap := &views.AssetProperties{}
	common.JSONUnmarshal(result.([]byte), ap)
	return ap, nil
}

func (c *Client) ReadAsset(id string) (*views.Asset, error) {
	result, err := c.c.CallView("ReadAsset", common.JSONMarshall(
		&views.ReadAsset{
			ID: id,
		},
	))
	if err != nil {
		return nil, err
	}
	asset := &views.Asset{}
	common.JSONUnmarshal(result.([]byte), asset)
	return asset, nil
}

func (c *Client) ChangePublicDescription(id string, publicDescription string) error {
	_, err := c.c.CallView("ChangePublicDescription", common.JSONMarshall(&views.ChangePublicDescription{
		ID:                id,
		PublicDescription: publicDescription,
	}))
	return err
}

func (c *Client) AgreeToSell(assetPrice *views.AssetPrice) error {
	_, err := c.c.CallView("AgreeToSell", common.JSONMarshall(assetPrice))
	return err
}

func (c *Client) AgreeToBuy(assetPrice *views.AssetPrice) error {
	_, err := c.c.CallView("AgreeToBuy", common.JSONMarshall(assetPrice))
	return err
}

func (c *Client) Transfer(assetProperties *views.AssetProperties, assetPrice *views.AssetPrice, recipient view.Identity) error {
	_, err := c.c.CallView("Transfer", common.JSONMarshall(&views.Transfer{
		Recipient:       recipient,
		AssetProperties: assetProperties,
		AssetPrice:      assetPrice,
	}))
	return err
}

func (c *Client) Identity() view.Identity {
	return c.id
}
