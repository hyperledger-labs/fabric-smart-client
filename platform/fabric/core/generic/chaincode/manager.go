/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type Manager struct {
	NetworkID       string
	ChannelID       string
	ConfigService   driver.ConfigService
	ChannelConfig   driver.ChannelConfig
	NumRetries      uint
	RetrySleep      time.Duration
	LocalMembership driver.LocalMembership
	PeerManager     Services
	SignerService   driver.SignerService
	Broadcaster     Broadcaster
	Finality        driver.Finality
	MSPProvider     MSPProvider

	// chaincodes
	ChaincodesLock sync.RWMutex
	Chaincodes     map[string]driver.Chaincode

	// ctx bounds the lifetime of resources (e.g. per-chaincode discovery
	// caches) created by this Manager. cancel is invoked by Stop.
	ctx    context.Context
	cancel context.CancelFunc
}

func NewManager(
	ctx context.Context,
	networkID string,
	channelID string,
	configService driver.ConfigService,
	channelConfig driver.ChannelConfig,
	numRetries uint,
	retrySleep time.Duration,
	localMembership driver.LocalMembership,
	peerManager Services,
	signerService driver.SignerService,
	broadcaster Broadcaster,
	finality driver.Finality,
	MSPProvider MSPProvider,
) *Manager {
	mctx, cancel := context.WithCancel(ctx)
	return &Manager{
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
		ctx:             mctx,
		cancel:          cancel,
	}
}

// Stop cancels the Manager's context, stopping all per-chaincode discovery
// caches (and any other resource whose lifetime is bound to it).
func (c *Manager) Stop() {
	c.cancel()
}

// Chaincode returns a chaincode handler for the passed chaincode name
func (c *Manager) Chaincode(name string) driver.Chaincode {
	c.ChaincodesLock.RLock()
	ch, ok := c.Chaincodes[name]
	c.ChaincodesLock.RUnlock()
	if ok {
		return ch
	}

	c.ChaincodesLock.Lock()
	defer c.ChaincodesLock.Unlock()
	ch, ok = c.Chaincodes[name]
	if ok {
		return ch
	}
	ch = NewChaincode(
		c.ctx,
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
