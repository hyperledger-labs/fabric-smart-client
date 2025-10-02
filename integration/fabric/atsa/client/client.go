/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/states"
	atsa "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ViewClient interface {
	CallView(fid string, in []byte) (interface{}, error)
}

type Client struct {
	id       view.Identity
	c        ViewClient
	approver view.Identity
}

func New(c ViewClient, id view.Identity, approver view.Identity) *Client {
	return &Client{c: c, id: id, approver: approver}
}

func (c *Client) Identity() view.Identity {
	return c.id
}

func (c *Client) Issue(asset *states.Asset) (string, error) {
	txIDBoxed, err := c.c.CallView("issue", common.JSONMarshall(&atsa.Issue{
		Asset:     asset,
		Recipient: asset.Owner,
		Approver:  c.approver,
	}))
	if err != nil {
		return "", err
	}
	return common.JSONUnmarshalString(txIDBoxed), err
}

func (c *Client) AgreeToSell(agreement *states.AgreementToSell) (string, error) {
	_, err := c.c.CallView("agreeToSell", common.JSONMarshall(&atsa.AgreeToSell{
		Agreement: agreement,
		Approver:  c.approver,
	}))
	if err != nil {
		return "", err
	}
	return agreement.GetLinearID()
}

func (c *Client) AgreeToBuy(agreement *states.AgreementToBuy) (string, error) {
	_, err := c.c.CallView("agreeToBuy", common.JSONMarshall(&atsa.AgreeToBuy{
		Agreement: agreement,
		Approver:  c.approver,
	}))
	if err != nil {
		return "", err
	}
	return agreement.GetLinearID()
}

func (c *Client) Transfer(assetID string, agreementID string, recipient view.Identity) error {
	_, err := c.c.CallView("transfer", common.JSONMarshall(&atsa.Transfer{
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

func (c *Client) IsTxFinal(id string) error {
	_, err := c.c.CallView("finality", common.JSONMarshall(cviews.Finality{TxID: id}))
	if err != nil {
		return err
	}
	return nil
}
