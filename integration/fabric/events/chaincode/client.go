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

func (c *Client) EventsView(chaincodeFunctions []string, eventCount uint8, eventName string) (interface{}, error) {
	event, err := c.c.CallView("EventsView", common.JSONMarshall(&views.Events{
		Functions:  chaincodeFunctions,
		EventCount: eventCount,
		EventName:  eventName,
	}))
	return event, err
}

func (c *Client) MultipleEventsView(chaincodeFunctions []string, eventCount uint8) (interface{}, error) {
	event, err := c.c.CallView("MultipleEventsView", common.JSONMarshall(&views.Events{
		Functions:  chaincodeFunctions,
		EventCount: eventCount,
	}))
	return event, err
}
