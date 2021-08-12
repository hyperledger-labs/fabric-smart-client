/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

type Chaincode struct {
	name    string
	sp      view.ServiceProvider
	network driver.FabricNetworkService
	channel Channel
}

func NewChaincode(name string, sp view.ServiceProvider, network driver.FabricNetworkService, channel Channel) *Chaincode {
	return &Chaincode{name: name, sp: sp, network: network, channel: channel}
}

func (c *Chaincode) NewInvocation(function string, args ...interface{}) driver.ChaincodeInvocation {
	return NewInvoke(c.sp, c.network, c.channel, c.name, function, args...)
}

func (c *Chaincode) NewDiscover() driver.ChaincodeDiscover {
	return NewDiscovery(c.network, c.channel, c.name)
}
