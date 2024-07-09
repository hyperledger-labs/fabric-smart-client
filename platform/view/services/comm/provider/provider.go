/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package provider

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
)

type P2PCommunicationType = string

const (
	LibP2P    P2PCommunicationType = "libp2p"
	WebSocket P2PCommunicationType = "websocket"
)

func NewHostProvider(config driver.ConfigService, endpointService *endpoint.Service, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) (host.GeneratorProvider, error) {
	if err := endpointService.AddPublicKeyExtractor(&comm.PKExtractor{}); err != nil {
		return nil, err
	}

	switch p2pCommType := config.GetString("fsc.p2p.type"); p2pCommType {
	case WebSocket:
		return newWebSocketHostProvider(config, endpointService, tracerProvider)
	case "":
		fallthrough
	case LibP2P:
		return newLibP2PHostProvider(endpointService, metricsProvider), nil
	default:
		return nil, errors.Errorf("fsc.p2p.type not supported: %s", p2pCommType)
	}
}

func newLibP2PHostProvider(endpointService *endpoint.Service, metricsProvider metrics.Provider) host.GeneratorProvider {
	endpointService.SetPublicKeyIDSynthesizer(&libp2p.PKIDSynthesizer{})
	return libp2p.NewHostGeneratorProvider(metricsProvider)
}

func newWebSocketHostProvider(config driver.ConfigService, endpointService *endpoint.Service, tracerProvider trace.TracerProvider) (host.GeneratorProvider, error) {
	routingConfigPath := config.GetPath("fsc.p2p.opts.routing.path")
	r, err := routing.NewResolvedStaticIDRouter(routingConfigPath, endpointService)
	if err != nil {
		return nil, err
	}
	discovery := routing.NewServiceDiscovery(r, routing.Random[host.PeerIPAddress]())
	endpointService.SetPublicKeyIDSynthesizer(&rest.PKIDSynthesizer{})
	return rest.NewEndpointBasedProvider(endpointService, discovery, tracerProvider), nil
}
