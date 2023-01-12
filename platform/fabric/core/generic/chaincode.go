/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// Chaincode returns a chaincode handler for the passed chaincode name
func (c *Channel) Chaincode(name string) driver.Chaincode {
	c.ChaincodesLock.RLock()
	ch, ok := c.Chaincodes[name]
	if ok {
		c.ChaincodesLock.RUnlock()
		return ch
	}
	c.ChaincodesLock.RUnlock()

	c.ChaincodesLock.Lock()
	defer c.ChaincodesLock.Unlock()
	ch, ok = c.Chaincodes[name]
	if ok {
		return ch
	}
	ch = chaincode.NewChaincode(name, c.SP, c.Network, c)
	c.Chaincodes[name] = ch
	return ch
}
