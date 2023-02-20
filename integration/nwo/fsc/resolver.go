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
	p.Resolvers = []*Resolver{}
	for _, peer := range p.Peers {
		org := p.Organization(peer.Organization)

		addresses := map[api.PortName]string{
			//ViewPort: fmt.Sprintf("127.0.0.1:%d", p.Context.PortsByPeerID("fsc", peer.ID())[ListenPort]),
			//ListenPort: fmt.Sprintf("127.0.0.1:%d", p.Context.PortsByPeerID("fsc", peer.ID())[ListenPort]),
		}
		if peer.Bootstrap {
			addresses[P2PPort] = fmt.Sprintf("%s:%d", p.Context.HostByPeerID("fsc", peer.ID()), p.Context.PortsByPeerID("fsc", peer.ID())[P2PPort])
		}

		p.Resolvers = append(p.Resolvers, &Resolver{
			Name: peer.Name,
			Identity: ResolverIdentity{
				ID:   peer.Name,
				Path: p.LocalMSPIdentityCert(peer),
			},
			Domain:    org.Domain,
			Addresses: addresses,
			Aliases:   peer.Aliases,
		})
	}
}
