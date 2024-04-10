/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type Broadcaster interface {
	Broadcast(context context.Context, blob interface{}) error
}

type MSPProvider interface {
	MSPManager() driver.MSPManager
}

type ChaincodeManager struct {
	NetworkID       string
	ChannelID       string
	ConfigService   driver.ConfigService
	ChannelConfig   driver.ChannelConfig
	NumRetries      uint
	RetrySleep      time.Duration
	LocalMembership driver.LocalMembership
	PeerManager     chaincode.PeerManager
	SignerService   driver.SignerService
	Broadcaster     Broadcaster
	Finality        driver.Finality
	MSPProvider     MSPProvider

	// chaincodes
	ChaincodesLock sync.RWMutex
	Chaincodes     map[string]driver.Chaincode
}

func NewChaincodeManager(
	networkID string,
	channelID string,
	configService driver.ConfigService,
	channelConfig driver.ChannelConfig,
	numRetries uint,
	retrySleep time.Duration,
	localMembership driver.LocalMembership,
	peerManager chaincode.PeerManager,
	signerService driver.SignerService,
	broadcaster Broadcaster,
	finality driver.Finality,
	MSPProvider MSPProvider,
) *ChaincodeManager {
	return &ChaincodeManager{
		NetworkID:       networkID,
		ChannelID:       channelID,
		ConfigService:   configService,
		ChannelConfig:   channelConfig,
		NumRetries:      numRetries,
		RetrySleep:      retrySleep,
		LocalMembership: localMembership,
		PeerManager:     peerManager,
		SignerService:   signerService,
		Broadcaster:     broadcaster,
		Finality:        finality,
		MSPProvider:     MSPProvider,
		Chaincodes:      map[string]driver.Chaincode{},
	}
}

// Chaincode returns a chaincode handler for the passed chaincode name
func (c *ChaincodeManager) Chaincode(name string) driver.Chaincode {
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
	ch = chaincode.NewChaincode(
		name,
		c.ConfigService,
		c.ChannelConfig,
		c.LocalMembership,
		c.PeerManager,
		c.SignerService,
		c.Broadcaster,
		c.Finality,
		c.MSPProvider,
	)
	c.Chaincodes[name] = ch
	return ch
}
