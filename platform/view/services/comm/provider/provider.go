/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package provider

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
)

func NewHostProvider(config driver.ConfigService, endpointService *endpoint.Service, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) (host.GeneratorProvider, error) {
	if err := endpointService.AddPublicKeyExtractor(&comm.PKExtractor{}); err != nil {
		return nil, err
	}

	if p2pCommType := config.GetString("fsc.p2p.type"); strings.EqualFold(p2pCommType, fsc.WebSocket) {
		return newWebSocketHostProvider(config, endpointService, tracerProvider, metricsProvider)
	} else {
		return newLibP2PHostProvider(config, endpointService, metricsProvider), nil
	}
}

func newLibP2PHostProvider(config driver.ConfigService, endpointService *endpoint.Service, metricsProvider metrics.Provider) host.GeneratorProvider {
	endpointService.SetPublicKeyIDSynthesizer(&libp2p.PKIDSynthesizer{})
	return libp2p.NewHostGeneratorProvider(libp2p.NewConfig(config), metricsProvider, view.NewEndpointService(endpointService))
}

func newWebSocketHostProvider(config driver.ConfigService, endpointService *endpoint.Service, tracerProvider trace.TracerProvider, metricsProvider metrics.Provider) (host.GeneratorProvider, error) {
	routingConfigPath := config.GetPath("fsc.p2p.opts.routing.path")
	r, err := routing.NewResolvedStaticIDRouter(routingConfigPath, endpointService)
	if err != nil {
		return nil, err
	}
	discovery := routing.NewServiceDiscovery(r, routing.Random[host.PeerIPAddress]())
	endpointService.SetPublicKeyIDSynthesizer(&rest.PKIDSynthesizer{})
	return rest.NewEndpointBasedProvider(rest.NewConfig(config), endpointService, discovery, websocket.NewMultiplexedProvider(tracerProvider, metricsProvider)), nil
}
