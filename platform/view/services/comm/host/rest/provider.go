/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"crypto/tls"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	routing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// P2PCommunicationType is a string identifier for the websocket implementation of the p2p comm stack.
const P2PCommunicationType = "websocket"

type pkiExtractor interface {
	ExtractPKI(id []byte) []byte
}

type endpointService interface {
	pkiExtractor
	Resolvers() []endpoint.ResolverInfo
	UpdateResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error)
}

type endpointServiceBasedProvider struct {
	config          Config
	endpointService endpointService
	routing         routing2.ServiceDiscovery
	streamProvider  StreamProvider
}

func NewEndpointBasedProvider(config Config, endpointService endpointService, routing routing2.ServiceDiscovery, streamProvider StreamProvider) *endpointServiceBasedProvider {
	return &endpointServiceBasedProvider{
		config:          config,
		endpointService: endpointService,
		routing:         routing,
		streamProvider:  streamProvider,
	}
}

func (p *endpointServiceBasedProvider) GetNewHost() (host2.P2PHost, error) {
	raw, err := id.LoadIdentity(p.config.CertPath())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load identity in [%s]", p.config.CertPath())
	}
	nodeID := string(p.endpointService.ExtractPKI(raw))
	address, err := comm.ConvertAddress(p.config.ListenAddress())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert address")
	}
	clientTLSConfig := p.config.ClientTLSConfig(p)
	serverTLSConfig := p.config.ServerTLSConfig(p)
	if clientTLSConfig == nil || serverTLSConfig == nil {
		return nil, errors.Errorf("websocket p2p communication requires TLS and mutual TLS configuration")
	}
	if serverTLSConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		return nil, errors.Errorf("websocket p2p communication requires mutual TLS (client certificates)")
	}

	return &hostWrapper{
		P2PHost:         NewHost(nodeID, address, p.routing, p.streamProvider, clientTLSConfig, serverTLSConfig),
		endpointService: p.endpointService,
		nodeID:          nodeID,
	}, nil
}

type hostWrapper struct {
	host2.P2PHost
	endpointService endpointService
	nodeID          string
}

func (h *hostWrapper) ID() string {
	return h.P2PHost.(interface{ ID() string }).ID()
}

func (h *hostWrapper) Addr() string {
	return h.P2PHost.(interface{ Addr() string }).Addr()
}

func (h *hostWrapper) Start(newStreamCallback func(stream host2.P2PStream)) error {
	if err := h.P2PHost.Start(newStreamCallback); err != nil {
		return err
	}

	// Update the endpoint service with the actual address
	actualAddr := h.P2PHost.(interface{ Addr() string }).Addr()
	logger.Infof("Updating endpoint service for node [%s] with actual address [%s]", h.nodeID, actualAddr)
	_, err := h.endpointService.UpdateResolver(
		h.nodeID,
		"",
		map[string]string{string(endpoint.P2PPort): actualAddr},
		nil,
		[]byte(h.nodeID),
	)
	if err != nil {
		logger.Errorf("failed to update endpoint service for node [%s]: %s", h.nodeID, err)
	}

	return nil
}

func (p *endpointServiceBasedProvider) ExtraCAs() [][]byte {
	var extraCAs [][]byte
	for _, resolver := range p.endpointService.Resolvers() {
		extraCAs = append(extraCAs, resolver.ID)
	}
	return extraCAs
}
