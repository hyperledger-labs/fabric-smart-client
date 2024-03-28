/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
)

type ResolverIdentity struct {
	ID   string
	Path string
}

type Resolver struct {
	Name      string
	Domain    string
	Identity  ResolverIdentity
	Addresses map[api.PortName]string
	Aliases   []string
	Port      int
}

func (p *Platform) GenerateResolverMap() {
	resolvers := make([]*Resolver, len(p.Peers))
	routing := make(map[string][]string)
	for i, peer := range p.Peers {
		org := p.Organization(peer.Organization)

		addresses := map[api.PortName]string{
			//ViewPort: fmt.Sprintf("127.0.0.1:%d", p.Context.PortsByPeerID("fsc", peer.ID())[ListenPort]),
			//ListenPort: fmt.Sprintf("127.0.0.1:%d", p.Context.PortsByPeerID("fsc", peer.ID())[ListenPort]),
		}
		p2pEndpoint := fmt.Sprintf("%s:%d", p.Context.HostByPeerID("fsc", peer.ID()), p.Context.PortsByPeerID("fsc", peer.ID())[P2PPort])
		if peer.Bootstrap {
			addresses[P2PPort] = p2pEndpoint
		}

		if peerRoutes, ok := routing[peer.Name]; ok {
			routing[peer.Name] = append(peerRoutes, p2pEndpoint)
		} else {
			routing[peer.Name] = []string{p2pEndpoint}
		}

		resolvers[i] = &Resolver{
			Name: peer.Name,
			Identity: ResolverIdentity{
				ID:   peer.Name,
				Path: p.LocalMSPIdentityCert(peer.Peer),
			},
			Domain:    org.Domain,
			Addresses: addresses,
			Aliases:   peer.Aliases,
		}
	}
	p.Resolvers = resolvers
	if p.P2PCommunicationType() == WebSocket {
		p.Routing = routing
	}
}
