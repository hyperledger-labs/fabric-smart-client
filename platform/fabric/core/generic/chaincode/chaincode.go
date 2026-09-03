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

// MSPProvider gives the chaincode package the channel's membership as a trust
// anchor: the MSPs the channel recognizes and the TLS certificates that
// authenticate its organizations' peers. Both must come from the channel
// configuration rather than from a discovery response — that response is
// supplied by whichever peer answered the query, so using it as its own trust
// anchor would let that peer authorize the identities and endpoints it reports.
//
// A channel's configuration arrives asynchronously, so both methods report
// [driver.ErrNotInitialized] while none has arrived and
// [driver.ErrConfigRejected] once one has arrived and been refused. Neither is a
// verdict on what was asked about, and a caller that treats them as one
// misreports a node that is merely still starting up. An implementation may
// wait a bounded time for the first configuration instead of reporting
// [driver.ErrNotInitialized] straight away, so a call on a freshly started node
// can block; a refusal is reported without waiting.
type MSPProvider interface {
	// MSPManager returns a manager that resolves serialized identities against
	// the channel's MSPs. It never returns nil and may be called before a
	// configuration is in force; that failure surfaces from the manager's own
	// methods. Callers should not memoize the result.
	MSPManager() driver.MSPManager
	// TLSRootCertsByMSPID returns the TLS root and intermediate certificates, in
	// that order, of the application organization with the given MSP ID. It
	// fails if the channel has no such organization.
	TLSRootCertsByMSPID(mspID string) ([][]byte, error)
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
