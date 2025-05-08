/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	routing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type pkiExtractor interface {
	ExtractPKI(id []byte) []byte
}

type endpointServiceBasedProvider struct {
	config         Config
	pkiExtractor   pkiExtractor
	routing        routing2.ServiceDiscovery
	tracerProvider trace.TracerProvider
	streamProvider StreamProvider
}

func NewEndpointBasedProvider(config Config, extractor pkiExtractor, routing routing2.ServiceDiscovery, tracerProvider trace.TracerProvider, streamProvider StreamProvider) *endpointServiceBasedProvider {
	return &endpointServiceBasedProvider{
		config:         config,
		pkiExtractor:   extractor,
		routing:        routing,
		tracerProvider: tracerProvider,
		streamProvider: streamProvider,
	}
}

func (p *endpointServiceBasedProvider) GetNewHost() (host2.P2PHost, error) {
	raw, err := id.LoadIdentity(p.config.CertPath())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load identity in [%s]", p.config.CertPath())
	}
	nodeID := string(p.pkiExtractor.ExtractPKI(raw))
	tlsConfig, err := p.config.TLSConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting new host for [%s]", p.config.ListenAddress())
	}
	return NewHost(nodeID, convertAddress(p.config.ListenAddress()), p.routing, p.tracerProvider, p.streamProvider, tlsConfig), nil
}

func convertAddress(addr string) string {
	parts := strings.Split(addr, "/")
	if len(parts) != 5 {
		panic("unexpected address found: " + addr)
	}
	return parts[2] + ":" + parts[4]
}
