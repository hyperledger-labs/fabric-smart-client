/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package routing

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type EndpointService interface {
	GetIdentity(endpoint string, pkID []byte) (view.Identity, error)
	GetResolver(ctx context.Context, party view.Identity) (*endpoint.Resolver, error)
}

// EndpointServiceIDRouter resolves the IP addresses using the resolvers of the endpoint service.
type EndpointServiceIDRouter struct {
	es EndpointService
}

func NewEndpointServiceIDRouter(es EndpointService) *EndpointServiceIDRouter {
	return &EndpointServiceIDRouter{es: es}
}

func (r *EndpointServiceIDRouter) Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	logger.Debugf("Looking up endpoint of peer [%s]", id)
	identity, err := r.es.GetIdentity("", []byte(id))
	if err != nil {
		logger.Errorf("failed getting identity for peer [%s]", id)
		return []host2.PeerIPAddress{}, false
	}
	resolver, err := r.es.GetResolver(context.Background(), identity)
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

// StaticIDRouter is a map implementation that contains all hard-coded routes
type StaticIDRouter map[host2.PeerID][]host2.PeerIPAddress

func (r StaticIDRouter) Lookup(id string) ([]host2.PeerIPAddress, bool) {
	addr, ok := r[id]
	return addr, ok
}

func (r StaticIDRouter) ReverseLookup(ipAddress host2.PeerIPAddress) (host2.PeerID, bool) {
	for id, addrs := range r {
		if slices.Contains(addrs, ipAddress) {
			return id, true
		}
	}
	return "", false
}

// ResolvedStaticIDRouter resolves the address of a peer ID by finding the label with the help of the endpoint service,
// and then using a LabelResolver to find the IPs of the peers that share this label.
type ResolvedStaticIDRouter struct {
	routes   LabelRouter
	resolver *LabelResolver
}

func NewResolvedStaticIDRouter(configPath string, es EndpointService) (*ResolvedStaticIDRouter, error) {
	labelRouting, err := newStaticLabelRouter(configPath)
	if err != nil {
		return nil, err
	}

	return &ResolvedStaticIDRouter{
		routes:   labelRouting,
		resolver: newLabelResolver(es),
	}, nil
}

func (r *ResolvedStaticIDRouter) Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	label, err := r.resolver.getLabel(id)
	if err != nil {
		logger.Errorf("failed to look up peer ID [%s]", id)
		return nil, false
	}
	addresses, ok := r.routes.Lookup(label)
	return addresses, ok
}

// LabelResolver resolves a peer ID into its label
type LabelResolver struct {
	es        EndpointService
	cache     map[host2.PeerID]string
	cacheLock sync.RWMutex
}

func newLabelResolver(es EndpointService) *LabelResolver {
	return &LabelResolver{
		es:    es,
		cache: make(map[host2.PeerID]string),
	}
}

func (r *LabelResolver) getLabel(peerID host2.PeerID) (string, error) {
	r.cacheLock.RLock()
	if label, ok := r.cache[peerID]; ok {
		r.cacheLock.RUnlock()
		return label, nil
	}
	r.cacheLock.RUnlock()
	r.cacheLock.Lock()
	defer r.cacheLock.Unlock()
	if label, ok := r.cache[peerID]; ok {
		return label, nil
	}
	logger.Debugf("Label not found in cache. Looking up in endpoint service.")
	identity, err := r.es.GetIdentity("", []byte(peerID))
	if identity == nil && err != nil {
		return "", errors.Wrapf(err, "failed to find identity for peer [%s]", peerID)
	}

	resolver, err := r.es.GetResolver(context.Background(), identity)
	if err != nil {
		return "", errors.Wrapf(err, "failed to resolve identity [%s]", identity)
	}
	label := strings.TrimPrefix(resolver.GetName(), "fsc.")
	r.cache[peerID] = label
	return label, nil
}
