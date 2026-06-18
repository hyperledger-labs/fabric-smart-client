/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package provider

import (
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
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
	p2pCommType := strings.ToLower(config.GetString("fsc.p2p.type"))
	switch p2pCommType {
	case websocket.P2PCommunicationType:
		return NewWebSocketHostProvider(config, endpointService, tracerProvider, metricsProvider)
	default:
		return nil, fmt.Errorf("unknown p2p type: %v", p2pCommType)
	}
}

func NewWebSocketHostProvider(config driver.ConfigService, endpointService *endpoint.Service, tracerProvider tracing.Provider, metricsProvider metrics.Provider) (host.GeneratorProvider, error) {
	if err := endpointService.AddPublicKeyExtractor(&comm.PKExtractor{}); err != nil {
		return nil, err
	}
	r := routing.NewEndpointServiceIDRouter(endpointService)
	discovery := routing.NewServiceDiscovery(r, routing.Random[host.PeerIPAddress]())
	endpointService.SetPublicKeyIDSynthesizer(&websocket.PKIDSynthesizer{})
	restConfig, err := websocket.NewConfig(config)
	if err != nil {
		return nil, err
	}
	return websocket.NewEndpointBasedProvider(restConfig, endpointService, discovery, ws.NewMultiplexedProvider(tracerProvider, metricsProvider, restConfig.MaxSubConns())), nil
}
