/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"os"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type routing interface {
	Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool)
}

type endpointResolver interface {
	GetIdentity(endpoint string, pkID []byte) (view2.Identity, error)
	Resolve(party view2.Identity) (string, view2.Identity, map[driver.PortName]string, []byte, error)
}

type endpointServiceRouting struct {
	resolver endpointResolver
}

func NewEndpointServiceRouting(resolver endpointResolver) *endpointServiceRouting {
	return &endpointServiceRouting{resolver: resolver}
}

func (r *endpointServiceRouting) Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	logger.Infof("Looking up endpoint of peer [%s]", id)
	identity, err := r.resolver.GetIdentity("", []byte(id))
	if err != nil {
		logger.Errorf("failed getting identity for peer [%s]", id)
		return []host2.PeerIPAddress{}, false
	}
	_, _, addresses, _, err := r.resolver.Resolve(identity)
	if err != nil {
		logger.Errorf("failed resolving [%s]: %v", id, err)
		return []host2.PeerIPAddress{}, false
	}
	if address, ok := addresses[driver.P2PPort]; ok {
		logger.Infof("Found endpoint of peer [%s]: [%s]", id, address)
		return []host2.PeerIPAddress{address}, true
	}
	logger.Infof("Did not find endpoint of peer [%s]", id)
	return []host2.PeerIPAddress{}, false
}

func convertAddress(addr string) string {
	parts := strings.Split(addr, "/")
	if len(parts) != 5 {
		panic("unexpected address found: " + addr)
	}
	return parts[2] + ":" + parts[4]
}

type StaticRouter map[host2.PeerID][]host2.PeerIPAddress

func (r StaticRouter) Lookup(id string) ([]host2.PeerIPAddress, bool) {
	addr, ok := r[id]
	return addr, ok
}

func (r StaticRouter) ReverseLookup(ipAddress host2.PeerIPAddress) (host2.PeerID, bool) {
	for id, addrs := range r {
		for _, addr := range addrs {
			if ipAddress == addr {
				return id, true
			}
		}
	}
	return "", false
}

type resolvedStaticRouter struct {
	routes   map[string][]host2.PeerIPAddress
	resolver *labelResolver
}

func NewResolvedStaticRouter(configPath string, resolver endpointResolver) (*resolvedStaticRouter, error) {
	bytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file")
	}
	wrapper := struct {
		Routes map[string][]host2.PeerIPAddress `yaml:"routes"`
	}{}
	if err := yaml.Unmarshal(bytes, &wrapper); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal config")
	}

	logger.Infof("Found routes: %v", wrapper.Routes)

	return &resolvedStaticRouter{
		routes:   wrapper.Routes,
		resolver: newLabelResolver(resolver),
	}, nil
}

func (r *resolvedStaticRouter) Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	label, err := r.resolver.getLabel(id)
	if err != nil {
		logger.Errorf("failed to look up peer ID [%s]", id)
		return nil, false
	}
	addresses, ok := r.routes[label]
	return addresses, ok
}

type labelResolver struct {
	resolver endpointResolver
	cache    map[host2.PeerID]string
}

func newLabelResolver(resolver endpointResolver) *labelResolver {
	return &labelResolver{
		resolver: resolver,
		cache:    make(map[host2.PeerID]string),
	}
}

func (r *labelResolver) getLabel(peerID host2.PeerID) (string, error) {
	if label, ok := r.cache[peerID]; ok {
		return label, nil
	}
	logger.Debugf("Label not found in cache. Looking up in endpoint service.")
	identity, err := r.resolver.GetIdentity("", []byte(peerID))
	if identity == nil && err != nil {
		return "", errors.Wrapf(err, "failed to find identity for peer [%s]", peerID)
	}

	label, _, _, pkid, err := r.resolver.Resolve(identity)
	if pkid == nil && err != nil {
		return "", errors.Wrapf(err, "failed to resolve identity [%s] for label [%s]", identity, label)
	}
	label = strings.TrimPrefix(label, "fsc.")
	r.cache[peerID] = label
	return label, nil
}
