/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	routing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const P2PCommunicationType = "grpc"

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
}

func NewEndpointBasedProvider(config Config, endpointService endpointService, routing routing2.ServiceDiscovery) *endpointServiceBasedProvider {
	return &endpointServiceBasedProvider{
		config:          config,
		endpointService: endpointService,
		routing:         routing,
	}
}

func (p *endpointServiceBasedProvider) GetNewHost() (host2.P2PHost, error) {
	raw, err := id.LoadIdentity(p.config.CertPath())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load identity in [%s]", p.config.CertPath())
	}
	nodeID := string(p.endpointService.ExtractPKI(raw))

	clientConfig, err := p.config.ClientConfig(p)
	if err != nil {
		return nil, err
	}
	serverConfig, err := p.config.ServerConfig(p)
	if err != nil {
		return nil, err
	}
	if !clientConfig.SecOpts.UseTLS || !serverConfig.SecOpts.UseTLS {
		return nil, errors.Errorf("grpc p2p communication requires TLS and mutual TLS configuration")
	}
	if !clientConfig.SecOpts.RequireClientCert || !serverConfig.SecOpts.RequireClientCert {
		return nil, errors.Errorf("grpc p2p communication requires mutual TLS (client certificates)")
	}
	if serverConfig.SecOpts.RequireClientCert && serverConfig.SecOpts.UseTLS && serverConfig.SecOpts.VerifyCertificate == nil && len(serverConfig.SecOpts.ClientRootCAs) == 0 {
		return nil, errors.Errorf("grpc p2p communication requires trusted client root CAs")
	}

	h, err := NewHost(nodeID, p.routing, clientConfig, serverConfig, p.config.ListenAddress())
	if err != nil {
		return nil, err
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
	return h.P2PHost.(interface{ ID() string }).ID()
}

func (h *hostWrapper) Addr() string {
	return h.P2PHost.(interface{ Addr() string }).Addr()
}

func (h *hostWrapper) Start(newStreamCallback func(stream host2.P2PStream)) error {
	if err := h.P2PHost.Start(newStreamCallback); err != nil {
		return err
	}

	actualAddr := h.P2PHost.(interface{ Addr() string }).Addr()
	_, err := h.endpointService.UpdateResolver(
		h.nodeID,
		"",
		map[string]string{string(endpoint.P2PPort): actualAddr},
		nil,
		[]byte(h.nodeID),
	)
	return err
}

func (p *endpointServiceBasedProvider) ExtraCAs() [][]byte {
	var extraCAs [][]byte
	for _, resolver := range p.endpointService.Resolvers() {
		extraCAs = append(extraCAs, resolver.ID)
	}
	return extraCAs
}

var _ host2.GeneratorProvider = (*endpointServiceBasedProvider)(nil)
