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
	discovery2 "github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/pkg/errors"
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
	response, err := NewDiscovery(c).Response()
	if err != nil {
		return "", errors.WithMessage(err, "failed to discover")
	}
	endorsers, err := response.ForChannel(c.channel.Name()).Endorsers([]*discovery2.ChaincodeCall{{
		Name: c.name,
	}}, &noFilter{})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get endorsers for chaincode [%s]", c.name)
	}
	if len(endorsers) == 0 {
		return "", errors.Errorf("no endorsers found for chaincode [%s]", c.name)
	}
	stateInfoMessage := endorsers[0].StateInfoMessage
	if stateInfoMessage == nil {
		return "", errors.Errorf("no state info message found for chaincode [%s]", c.name)
	}
	stateInfo := stateInfoMessage.GetStateInfo()
	if stateInfo == nil {
		return "", errors.Errorf("no state info found for chaincode [%s]", c.name)
	}
	properties := stateInfo.GetProperties()
	if properties == nil {
		return "", errors.Errorf("no properties found for chaincode [%s]", c.name)
	}
	chaincodes := properties.Chaincodes
	if len(chaincodes) == 0 {
		return "", errors.Errorf("no chaincode info found for chaincode [%s]", c.name)
	}
	for _, chaincode := range chaincodes {
		if chaincode.Name == c.name {
			return chaincode.Version, nil
		}
	}
	return "", errors.Errorf("chaincode [%s] not found", c.name)
}
