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
func (c *channel) Chaincode(name string) driver.Chaincode {
	c.chaincodesLock.RLock()
	ch, ok := c.chaincodes[name]
	if ok {
		c.chaincodesLock.RUnlock()
		return ch
	}
	c.chaincodesLock.RUnlock()

	c.chaincodesLock.Lock()
	defer c.chaincodesLock.Unlock()
	ch, ok = c.chaincodes[name]
	if ok {
		return ch
	}
	ch = chaincode.NewChaincode(name, c.sp, c.network, c)
	c.chaincodes[name] = ch
	return ch
}
