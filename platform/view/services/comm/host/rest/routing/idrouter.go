/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package routing

import (
	"strings"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type endpointService interface {
	GetIdentity(endpoint string, pkID []byte) (view2.Identity, error)
	Resolve(party view2.Identity) (string, view2.Identity, map[driver.PortName]string, []byte, error)
}

// endpointServiceIDRouter resolves the IP addresses using the resolvers of the endpoint service.
type endpointServiceIDRouter struct {
	es endpointService
}

func NewEndpointServiceIDRouter(es endpointService) *endpointServiceIDRouter {
	return &endpointServiceIDRouter{es: es}
}

func (r *endpointServiceIDRouter) Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	logger.Debugf("Looking up endpoint of peer [%s]", id)
	identity, err := r.es.GetIdentity("", []byte(id))
	if err != nil {
		logger.Errorf("failed getting identity for peer [%s]", id)
		return []host2.PeerIPAddress{}, false
	}
	_, _, addresses, _, err := r.es.Resolve(identity)
	if err != nil {
		logger.Errorf("failed resolving [%s]: %v", id, err)
		return []host2.PeerIPAddress{}, false
	}
	if address, ok := addresses[driver.P2PPort]; ok {
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
		for _, addr := range addrs {
			if ipAddress == addr {
				return id, true
			}
		}
	}
	return "", false
}

// resolvedStaticIDRouter resolves the address of a peer ID by finding the label with the help of the endpoint service,
// and then using a labelResolver to find the IPs of the peers that share this label.
type resolvedStaticIDRouter struct {
	routes   LabelRouter
	resolver *labelResolver
}

func NewResolvedStaticIDRouter(configPath string, es endpointService) (*resolvedStaticIDRouter, error) {
	labelRouting, err := newStaticLabelRouter(configPath)
	if err != nil {
		return nil, err
	}

	return &resolvedStaticIDRouter{
		routes:   labelRouting,
		resolver: newLabelResolver(es),
	}, nil
}

func (r *resolvedStaticIDRouter) Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	label, err := r.resolver.getLabel(id)
	if err != nil {
		logger.Errorf("failed to look up peer ID [%s]", id)
		return nil, false
	}
	addresses, ok := r.routes.Lookup(label)
	return addresses, ok
}

// labelResolver resolves a peer ID into its label
type labelResolver struct {
	es        endpointService
	cache     map[host2.PeerID]string
	cacheLock sync.RWMutex
}

func newLabelResolver(es endpointService) *labelResolver {
	return &labelResolver{
		es:    es,
		cache: make(map[host2.PeerID]string),
	}
}

func (r *labelResolver) getLabel(peerID host2.PeerID) (string, error) {
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

	label, _, _, pkid, err := r.es.Resolve(identity)
	if pkid == nil && err != nil {
		return "", errors.Wrapf(err, "failed to resolve identity [%s] for label [%s]", identity, label)
	}
	label = strings.TrimPrefix(label, "fsc.")
	r.cache[peerID] = label
	return label, nil
}
