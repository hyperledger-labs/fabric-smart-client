/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"crypto/tls"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	routing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/routing"
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
	UpdateResolver(name, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error)
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

// GetNewHost builds a new websocket P2P host, deriving the node ID from the configured
// identity and validating that mutual TLS is configured before constructing it. It returns an
// error if the identity cannot be loaded, the TLS configuration is missing or not mutual, or the
// host itself fails to build.
func (p *endpointServiceBasedProvider) GetNewHost() (host2.P2PHost, error) {
	raw, err := id.LoadIdentity(p.config.CertPath())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load identity in [%s]", p.config.CertPath())
	}
	nodeID := string(p.endpointService.ExtractPKI(raw))
	clientTLSConfig, err := p.config.ClientTLSConfig(p)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build client TLS config")
	}
	serverTLSConfig, err := p.config.ServerTLSConfig(p)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build server TLS config")
	}
	if clientTLSConfig == nil || serverTLSConfig == nil {
		return nil, errors.Errorf("websocket p2p communication requires TLS and mutual TLS configuration")
	}
	if serverTLSConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		return nil, errors.Errorf("websocket p2p communication requires mutual TLS (client certificates)")
	}

	h, err := NewHost(nodeID, p.routing, p.streamProvider, p.config, p)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create p2p host")
	}
	return &hostWrapper{
		P2PHost:         h,
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
	idHost, ok := h.P2PHost.(interface{ ID() string })
	if !ok {
		panic(errors.Errorf("unexpected P2PHost type [%T]: missing ID()", h.P2PHost))
	}
	return idHost.ID()
}

func (h *hostWrapper) Addr() string {
	addrHost, ok := h.P2PHost.(interface{ Addr() string })
	if !ok {
		panic(errors.Errorf("unexpected P2PHost type [%T]: missing Addr()", h.P2PHost))
	}
	return addrHost.Addr()
}

func (h *hostWrapper) Start(newStreamCallback func(stream host2.P2PStream)) error {
	if err := h.P2PHost.Start(newStreamCallback); err != nil {
		return err
	}

	// Update the endpoint service with the actual address
	addrHost, ok := h.P2PHost.(interface{ Addr() string })
	if !ok {
		panic(errors.Errorf("unexpected P2PHost type [%T]: missing Addr()", h.P2PHost))
	}
	actualAddr := addrHost.Addr()
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
