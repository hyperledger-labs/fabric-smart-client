/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ViewClient interface {
	CallView(fid string, in []byte) (interface{}, error)
	IsTxFinal(txid string, opts ...api.ServiceOption) error
}

type Client struct {
	id view.Identity
	c  ViewClient
}

func NewClient(c ViewClient, id view.Identity) *Client {
	return &Client{c: c, id: id}
}

func (c *Client) CreateAsset(ap *views.Asset) error {
	_, err := c.c.CallView("CreateAssetData", common.JSONMarshall(&views.CreateAsset{
		Asset: views.Asset{
			AppraisedValue: 100,
			Color:          "blue",
			ID:             "xyz",
			Owner:          "alice",
			Size:           10,
		},
	}))

	if err != nil {
		return err
	}
	return nil
}
