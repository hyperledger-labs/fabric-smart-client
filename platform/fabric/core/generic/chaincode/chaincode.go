/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"sync"

	"github.com/ReneKroon/ttlcache/v2"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

type Chaincode struct {
	name    string
	sp      view.ServiceProvider
	network Network
	channel Channel

	discoveryResultsCacheLock sync.RWMutex
	discoveryResultsCache     ttlcache.SimpleCache
}

func NewChaincode(name string, sp view.ServiceProvider, network Network, channel Channel) *Chaincode {
	return &Chaincode{
		name:                  name,
		sp:                    sp,
		network:               network,
		channel:               channel,
		discoveryResultsCache: ttlcache.NewCache(),
	}
}

func (c *Chaincode) NewInvocation(function string, args ...interface{}) driver.ChaincodeInvocation {
	return NewInvoke(c, function, args...)
}

func (c *Chaincode) NewDiscover() driver.ChaincodeDiscover {
	return NewDiscovery(c)
}

func (c *Chaincode) IsAvailable() (bool, error) {
	ids, err := c.NewDiscover().Call()
	if err != nil {
		return false, err
	}
	return len(ids) != 0, nil
}

func (c *Chaincode) IsPrivate() bool {
	channels, err := c.network.Config().Channels()
	if err != nil {
		logger.Error("failed getting channels' configurations [%s]", err)
		return false
	}
	for _, channel := range channels {
		if channel.Name == c.channel.Name() {
			for _, chaincode := range channel.Chaincodes {
				if chaincode.Name == c.name {
					return chaincode.Private
				}
			}
		}
	}
	// Nothing was found
	return false
}

// Version returns the version of this chaincode.
// It uses discovery to extract this information from the endorsers
func (c *Chaincode) Version() (string, error) {
	return NewDiscovery(c).ChaincodeVersion()
}
