/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package provider

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	grpccomm "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/ws"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

func NewHostProvider(
	config driver.ConfigService,
	endpointService *endpoint.Service,
	metricsProvider metrics.Provider,
	tracerProvider tracing.Provider,
) (host.GeneratorProvider, error) {
	if err := endpointService.AddPublicKeyExtractor(&comm.PKExtractor{}); err != nil {
		return nil, err
	}

	if p2pCommType := config.GetString("fsc.p2p.type"); strings.EqualFold(p2pCommType, websocket.P2PCommunicationType) {
		return NewWebSocketHostProvider(config, endpointService, tracerProvider, metricsProvider)
	}
	if p2pCommType := config.GetString("fsc.p2p.type"); strings.EqualFold(p2pCommType, grpccomm.P2PCommunicationType) {
		return NewGRPCHostProvider(config, endpointService)
	}

	return NewLibP2PHostProvider(config, endpointService, metricsProvider), nil
}

func NewLibP2PHostProvider(config driver.ConfigService, endpointService *endpoint.Service, metricsProvider metrics.Provider) host.GeneratorProvider {
	endpointService.SetPublicKeyIDSynthesizer(&libp2p.PKIDSynthesizer{})
	return libp2p.NewHostGeneratorProvider(libp2p.NewConfig(config), metricsProvider, endpointService)
}

func NewWebSocketHostProvider(config driver.ConfigService, endpointService *endpoint.Service, tracerProvider tracing.Provider, metricsProvider metrics.Provider) (host.GeneratorProvider, error) {
	r := routing.NewEndpointServiceIDRouter(endpointService)
	discovery := routing.NewServiceDiscovery(r, routing.Random[host.PeerIPAddress]())
	endpointService.SetPublicKeyIDSynthesizer(&websocket.PKIDSynthesizer{})
	restConfig, err := websocket.NewConfig(config)
	if err != nil {
		return nil, err
	}
	return websocket.NewEndpointBasedProvider(restConfig, endpointService, discovery, ws.NewMultiplexedProvider(tracerProvider, metricsProvider, restConfig.MaxSubConns())), nil
}

func NewGRPCHostProvider(config driver.ConfigService, endpointService *endpoint.Service) (host.GeneratorProvider, error) {
	r := routing.NewEndpointServiceIDRouter(endpointService)
	discovery := routing.NewServiceDiscovery(r, routing.Random[host.PeerIPAddress]())
	endpointService.SetPublicKeyIDSynthesizer(&grpccomm.PKIDSynthesizer{})
	grpcConfig, err := grpccomm.NewConfig(config)
	if err != nil {
		return nil, err
	}
	return grpccomm.NewEndpointBasedProvider(grpcConfig, endpointService, discovery), nil
}
