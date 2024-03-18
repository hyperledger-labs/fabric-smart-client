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
)

type pkiExtractor interface {
	ExtractPKI(id []byte) []byte
}

type endpointServiceBasedProvider struct {
	pkiExtractor pkiExtractor
	routing      routing2.IDRouter
}

func NewEndpointBasedProvider(extractor pkiExtractor, routing routing2.IDRouter) *endpointServiceBasedProvider {
	return &endpointServiceBasedProvider{
		pkiExtractor: extractor,
		routing:      routing,
	}
}

func (p *endpointServiceBasedProvider) NewBootstrapHost(listenAddress host2.PeerIPAddress, privateKeyPath, certPath string) (host2.P2PHost, error) {
	raw, err := id.LoadIdentity(certPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load identity in [%s]", certPath)
	}
	nodeID := string(p.pkiExtractor.ExtractPKI(raw))
	return NewHost(nodeID, convertAddress(listenAddress), p.routing, privateKeyPath, certPath, nil)
}

func (p *endpointServiceBasedProvider) NewHost(listenAddress host2.PeerIPAddress, privateKeyPath, certPath string, _ host2.PeerIPAddress) (host2.P2PHost, error) {
	return p.NewBootstrapHost(listenAddress, privateKeyPath, certPath)
}

func convertAddress(addr string) string {
	parts := strings.Split(addr, "/")
	if len(parts) != 5 {
		panic("unexpected address found: " + addr)
	}
	return parts[2] + ":" + parts[4]
}
