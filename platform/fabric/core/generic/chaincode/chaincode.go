/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/discovery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

var logger = logging.MustGetLogger()

type Services interface {
	NewPeerClient(cc grpc.ConnectionConfig) (services.PeerClient, error)
}

type Broadcaster interface {
	Broadcast(ctx context.Context, blob any) error
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

	// discoveryResultsCache caches discovery.Response objects across the
	// lifetime of the Chaincode. It is created once here (instead of once
	// per NewDiscovery call) so its background eviction goroutine is bound
	// to the Chaincode's ctx rather than leaking on every discovery.
	discoveryResultsCache cache.Map[string, discovery.Response]
}

// NewChaincode creates a Chaincode handler. networkConfig and channelConfig are
// required and must not be nil.
//
// Discovery results are cached for the lifetime of the returned Chaincode.
// The TTL of the cached results can be set via the `ChannelConfig.DiscoveryTimeout()`
// of the channelConfig parameter. If not set, the default `DiscoveryCacheTimeout` is used.
// The cache's background eviction goroutine is stopped when ctx is cancelled.
func NewChaincode(
	ctx context.Context,
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
	timeout := DiscoveryCacheTimeout
	if channelConfig.DiscoveryTimeout() > 0 {
		timeout = channelConfig.DiscoveryDefaultTTLS()
	}

	return &Chaincode{
		name:                  name,
		NetworkID:             networkConfig.NetworkName(),
		ChannelID:             channelConfig.ID(),
		ConfigService:         networkConfig,
		ChannelConfig:         channelConfig,
		NumRetries:            channelConfig.GetNumRetries(),
		RetrySleep:            channelConfig.GetRetrySleep(),
		LocalMembership:       localMembership,
		Services:              peerManager,
		SignerService:         signerService,
		Broadcaster:           broadcaster,
		Finality:              finality,
		MSPProvider:           MSPProvider,
		discoveryResultsCache: cache.NewTimeoutCache[string, discovery.Response](ctx, timeout, nil),
	}
}

func (c *Chaincode) NewInvocation(function string, args ...any) driver.ChaincodeInvocation {
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

// Version returns the version of this chaincode.
// It uses discovery to extract this information from the endorsers
func (c *Chaincode) Version() (string, error) {
	return NewDiscovery(c).ChaincodeVersion()
}
