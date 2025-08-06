/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	routing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
)

type pkiExtractor interface {
	ExtractPKI(id []byte) []byte
}

type endpointServiceBasedProvider struct {
	config         Config
	pkiExtractor   pkiExtractor
	routing        routing2.ServiceDiscovery
	streamProvider StreamProvider
}

func NewEndpointBasedProvider(config Config, extractor pkiExtractor, routing routing2.ServiceDiscovery, streamProvider StreamProvider) *endpointServiceBasedProvider {
	return &endpointServiceBasedProvider{
		config:         config,
		pkiExtractor:   extractor,
		routing:        routing,
		streamProvider: streamProvider,
	}
}

func (p *endpointServiceBasedProvider) GetNewHost() (host2.P2PHost, error) {
	raw, err := id.LoadIdentity(p.config.CertPath())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load identity in [%s]", p.config.CertPath())
	}
	nodeID := string(p.pkiExtractor.ExtractPKI(raw))
	return NewHost(nodeID, convertAddress(p.config.ListenAddress()), p.routing, p.streamProvider, p.config.ClientTLSConfig(), p.config.ServerTLSConfig()), nil
}

func convertAddress(addr string) string {
	parts := strings.Split(addr, "/")
	if len(parts) != 5 {
		panic("unexpected address found: " + addr)
	}
	return parts[2] + ":" + parts[4]
}
