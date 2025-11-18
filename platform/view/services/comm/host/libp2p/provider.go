/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	metrics2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// P2PCommunicationType is a string identifier for the libp2p implementation of the p2p comm stack.
const P2PCommunicationType = "libp2p"

type libp2pConfig interface {
	PrivateKeyPath() string
	Bootstrap() bool
	ListenAddress() host2.PeerIPAddress
	BootstrapListenAddress() host2.PeerIPAddress
}

type configService interface {
	GetString(key string) string
	GetPath(key string) string
}

func (c *config) PrivateKeyPath() string                      { return c.privateKeyPath }
func (c *config) Bootstrap() bool                             { return len(c.bootstrapListenAddress) == 0 }
func (c *config) ListenAddress() host2.PeerIPAddress          { return c.listenAddress }
func (c *config) BootstrapListenAddress() host2.PeerIPAddress { return c.bootstrapListenAddress }

type config struct {
	listenAddress          host2.PeerIPAddress
	bootstrapListenAddress host2.PeerIPAddress
	privateKeyPath         string
}

func NewConfig(cs configService) *config {
	return &config{
		listenAddress:          cs.GetString("fsc.p2p.listenAddress"),
		bootstrapListenAddress: cs.GetString("fsc.p2p.opts.bootstrapNode"),
		privateKeyPath:         cs.GetPath("fsc.identity.key.file"),
	}
}

type endpointService interface {
	Resolve(ctx context.Context, party view.Identity) (view.Identity, map[endpoint.PortName]string, []byte, error)
	GetIdentity(label string, pkID []byte) (view.Identity, error)
}

type hostGeneratorProvider struct {
	metrics         *metrics
	config          libp2pConfig
	endpointService endpointService
}

func NewHostGeneratorProvider(config libp2pConfig, provider metrics2.Provider, endpointService endpointService) *hostGeneratorProvider {
	return &hostGeneratorProvider{
		metrics:         newMetrics(provider),
		config:          config,
		endpointService: endpointService,
	}
}

func (p *hostGeneratorProvider) GetNewHost() (host2.P2PHost, error) {
	k, err := newCryptoPrivKeyFromMSP(p.config.PrivateKeyPath())
	if err != nil {
		return nil, err
	}
	if p.config.Bootstrap() {
		return newLibP2PHost(p.config.ListenAddress(), k, p.metrics, true, "")
	}

	bootstrap, err := p.getPeerAddress(p.config.BootstrapListenAddress())
	if err != nil {
		return nil, err
	}
	return newLibP2PHost(p.config.ListenAddress(), k, p.metrics, false, bootstrap)
}

func (p *hostGeneratorProvider) getPeerAddress(address host2.PeerIPAddress) (host2.PeerIPAddress, error) {
	bootstrapNodeID, err := p.endpointService.GetIdentity(address, nil)
	if err != nil {
		return "", errors.WithMessagef(err, "failed to get p2p bootstrap node's resolver entry [%s]", address)
	}
	_, endpoints, pkID, err := p.endpointService.Resolve(context.Background(), bootstrapNodeID)
	if err != nil {
		return "", errors.WithMessagef(err, "failed to resolve bootstrap node id [%s:%s]", address, bootstrapNodeID)
	}

	bootstrap, err := utils.AddressToEndpoint(endpoints[endpoint.P2PPort])
	if err != nil {
		return "", errors.WithMessagef(err, "failed to get the endpoint of the bootstrap node from [%s:%s], [%s]", address, bootstrapNodeID, endpoints[endpoint.P2PPort])
	}
	return bootstrap + "/p2p/" + string(pkID), nil
}
