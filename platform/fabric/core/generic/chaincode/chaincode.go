/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

var logger = logging.MustGetLogger("fabric-sdk.core.generic.chaincode")

type Services interface {
	NewPeerClient(cc grpc.ConnectionConfig) (services.PeerClient, error)
}

type Broadcaster interface {
	Broadcast(context context.Context, blob interface{}) error
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
	ConfigService   driver.ConfigService
	ChannelConfig   driver.ChannelConfig
	NumRetries      uint
	RetrySleep      time.Duration
	LocalMembership driver.LocalMembership
	Services        Services
	SignerService   driver.SignerService
	Broadcaster     Broadcaster
	Finality        driver.Finality
	MSPProvider     MSPProvider
}

func NewChaincode(
	name string,
	networkConfig driver.ConfigService,
	channelConfig driver.ChannelConfig,
	localMembership driver.LocalMembership,
	peerManager Services,
	signerService driver.SignerService,
	broadcaster Broadcaster,
	finality driver.Finality,
	MSPProvider MSPProvider,
) *Chaincode {
	return &Chaincode{
		name:            name,
		NetworkID:       networkConfig.NetworkName(),
		ChannelID:       channelConfig.ID(),
		ConfigService:   networkConfig,
		ChannelConfig:   channelConfig,
		NumRetries:      channelConfig.GetNumRetries(),
		RetrySleep:      channelConfig.GetRetrySleep(),
		LocalMembership: localMembership,
		Services:        peerManager,
		SignerService:   signerService,
		Broadcaster:     broadcaster,
		Finality:        finality,
		MSPProvider:     MSPProvider,
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
	channel := c.ConfigService.Channel(c.ChannelID)
	if channel == nil {
		return false
	}
	for _, chaincode := range channel.ChaincodeConfigs() {
		if chaincode.ID() == c.name {
			return chaincode.IsPrivate()
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
