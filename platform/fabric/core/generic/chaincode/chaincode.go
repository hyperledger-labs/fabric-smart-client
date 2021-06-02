/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package chaincode

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

type Chaincode struct {
	name    string
	sp      view.ServiceProvider
	network api.FabricNetworkService
	channel Channel
}

func NewChaincode(name string, sp view.ServiceProvider, network api.FabricNetworkService, channel Channel) *Chaincode {
	return &Chaincode{name: name, sp: sp, network: network, channel: channel}
}

func (c *Chaincode) NewInvocation(typ api.ChaincodeInvocationType, function string, args ...interface{}) api.ChaincodeInvocation {
	switch typ {
	case api.ChaincodeInvoke:
		return NewInvoke(c.sp, c.network, c.channel, c.name, function, args...)
	case api.ChaincodeQuery:
		return NewQuery(c.sp, c.network, c.channel, c.name, function, args...)
	case api.ChaincodeEndorse:
		return NewEndorse(c.sp, c.network, c.channel, c.name, function, args...)
	default:
		panic(fmt.Sprintf("invalid invocation type [%d]", typ))
	}
}

func (c *Chaincode) NewDiscover() api.ChaincodeDiscover {
	return NewDiscovery(c.network, c.channel, c.name)
}
