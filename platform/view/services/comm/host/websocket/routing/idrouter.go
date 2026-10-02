/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package routing

import (
	"context"
	"slices"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// EndpointService is the subset of the endpoint service that EndpointServiceIDRouter
// uses to map a peer ID to an identity and the identity to its resolver.
type EndpointService interface {
	GetIdentity(endpoint string, pkID []byte) (view.Identity, error)
	GetResolver(ctx context.Context, party view.Identity) (*endpoint.Resolver, error)
}

// EndpointServiceIDRouter resolves a peer ID to the P2P address of the endpoint
// service resolver bound to the peer's identity.
type EndpointServiceIDRouter struct {
	es EndpointService
}

func NewEndpointServiceIDRouter(es EndpointService) *EndpointServiceIDRouter {
	return &EndpointServiceIDRouter{es: es}
}

func (r *EndpointServiceIDRouter) Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	return r.LookupWithContext(context.Background(), id)
}

func (r *EndpointServiceIDRouter) LookupWithContext(ctx context.Context, id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	logger.Debugf("Looking up endpoint of peer [%s]", id)
	identity, err := r.es.GetIdentity("", []byte(id))
	if err != nil {
		logger.Errorf("failed getting identity for peer [%s]", id)
		return []host2.PeerIPAddress{}, false
	}
	resolver, err := r.es.GetResolver(ctx, identity)
	if err != nil {
		logger.Errorf("failed resolving [%s]: %s", id, err.Error())
		return []host2.PeerIPAddress{}, false
	}
	if address := resolver.GetAddress(endpoint.P2PPort); len(address) > 0 {
		logger.Debugf("Found endpoint of peer [%s]: [%s]", id, address)
		return []host2.PeerIPAddress{address}, true
	}
	logger.Debugf("Did not find endpoint of peer [%s]", id)
	return []host2.PeerIPAddress{}, false
}

// StaticIDRouter maps each peer ID to a fixed set of addresses.
type StaticIDRouter map[host2.PeerID][]host2.PeerIPAddress

func (r StaticIDRouter) Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	addr, ok := r[id]
	return addr, ok
}

// ReverseLookup returns a peer ID whose addresses include ipAddress.
func (r StaticIDRouter) ReverseLookup(ipAddress host2.PeerIPAddress) (host2.PeerID, bool) {
	for id, addrs := range r {
		if slices.Contains(addrs, ipAddress) {
			return id, true
		}
	}
	return "", false
}
