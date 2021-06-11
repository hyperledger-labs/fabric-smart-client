/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fsc

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ViewClient interface {
	CallView(fid string, in []byte) (interface{}, error)
	IsTxFinal(txid string) error
}

type Client struct {
	id       view.Identity
	c        ViewClient
	approver view.Identity
}

func NewClient(c ViewClient, id view.Identity, approver view.Identity) *Client {
	return &Client{c: c, id: id, approver: approver}
}

func (c *Client) Identity() view.Identity {
	return c.id
}

func (c *Client) Issue(asset *views.Asset) (string, error) {
	_, err := c.c.CallView("issue", common.JSONMarshall(&views.Issue{
		Asset:     asset,
		Recipient: asset.Owner,
		Approver:  c.approver,
	}))
	if err != nil {
		return "", err
	}
	return asset.GetLinearID()
}

func (c *Client) AgreeToSell(agreement *views.AgreementToSell) (string, error) {
	_, err := c.c.CallView("agreeToSell", common.JSONMarshall(&views.AgreeToSell{
		Agreement: agreement,
		Approver:  c.approver,
	}))
	if err != nil {
		return "", err
	}
	return agreement.GetLinearID()
}

func (c *Client) AgreeToBuy(agreement *views.AgreementToBuy) (string, error) {
	_, err := c.c.CallView("agreeToBuy", common.JSONMarshall(&views.AgreeToBuy{
		Agreement: agreement,
		Approver:  c.approver,
	}))
	if err != nil {
		return "", err
	}
	return agreement.GetLinearID()
}

func (c *Client) Transfer(assetID string, agreementID string, recipient view.Identity) error {
	_, err := c.c.CallView("transfer", common.JSONMarshall(&views.Transfer{
		AssetId:     assetID,
		AgreementId: agreementID,
		Recipient:   recipient,
		Approver:    c.approver,
	}))
	if err != nil {
		return err
	}
	return nil
}
