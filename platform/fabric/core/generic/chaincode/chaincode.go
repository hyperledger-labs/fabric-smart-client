/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/ReneKroon/ttlcache/v2"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

var logger = flogging.MustGetLogger("fabric-sdk.core.generic.chaincode")

type PeerManager interface {
	NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer.Client, error)
	PickPeer(funcType driver.PeerFunctionType) *grpc.ConnectionConfig
}

type Broadcaster interface {
	Broadcast(context context.Context, blob interface{}) error
}

type Finality interface {
	IsFinal(ctx context.Context, txID string) error
}

type SerializableSigner interface {
	Sign(message []byte) ([]byte, error)

	Serialize() ([]byte, error)
}

type MSPProvider interface {
	MSPManager() driver.MSPManager
}

type Chaincode struct {
	name            string
	NetworkID       string
	ChannelID       string
	NetworkConfig   *config.Config
	ChannelConfig   *config.Channel
	NumRetries      uint
	RetrySleep      time.Duration
	LocalMembership driver.LocalMembership
	PeerManager     PeerManager
	SignerService   driver.SignerService
	Broadcaster     Broadcaster
	Finality        Finality
	MSPProvider     MSPProvider

	discoveryResultsCacheLock sync.RWMutex
	discoveryResultsCache     ttlcache.SimpleCache
}

func NewChaincode(
	name string,
	networkConfig *config.Config,
	channelConfig *config.Channel,
	localMembership driver.LocalMembership,
	peerManager PeerManager,
	signerService driver.SignerService,
	broadcaster Broadcaster,
	finality Finality,
	MSPProvider MSPProvider,
) *Chaincode {
	return &Chaincode{
		name:                      name,
		NetworkID:                 networkConfig.Name(),
		ChannelID:                 channelConfig.Name,
		NetworkConfig:             networkConfig,
		ChannelConfig:             channelConfig,
		NumRetries:                channelConfig.NumRetries,
		RetrySleep:                channelConfig.RetrySleep,
		LocalMembership:           localMembership,
		PeerManager:               peerManager,
		SignerService:             signerService,
		Broadcaster:               broadcaster,
		Finality:                  finality,
		MSPProvider:               MSPProvider,
		discoveryResultsCacheLock: sync.RWMutex{},
		discoveryResultsCache:     ttlcache.NewCache(),
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
	channels, err := c.NetworkConfig.Channels()
	if err != nil {
		logger.Error("failed getting channels' configurations [%s]", err)
		return false
	}
	for _, channel := range channels {
		if channel.Name == c.ChannelID {
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
