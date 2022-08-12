/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

type ResolverIdentity struct {
	ID      string
	MSPType string
	MSPID   string
	Path    string
}

type Resolver struct {
	Name      string
	Domain    string
	Identity  ResolverIdentity
	Addresses map[api.PortName]string
	Port      int
	Aliases   []string
}

// ResolverMapPath returns the path to the generated resolver map configuration
// file.
func (n *Network) ResolverMapPath(p *topology.Peer) string {
	return filepath.Join(n.PeerDir(p), "resolver.json")
}

func (n *Network) GenerateResolverMap() {
	n.Resolvers = []*Resolver{}
	for _, peer := range n.Peers {
		org := n.Organization(peer.Organization)

		var addresses map[api.PortName]string
		var path string
		if peer.Type == topology.FSCPeer {
			if n.topology.NodeOUs {
				switch peer.Role {
				case "":
					path = n.PeerUserLocalMSPIdentityCert(peer, peer.Name)
				case "client":
					path = n.PeerUserLocalMSPIdentityCert(peer, peer.Name)
				default:
					path = n.PeerLocalMSPIdentityCert(peer)
				}
			} else {
				path = n.PeerUserLocalMSPIdentityCert(peer, peer.Name)
			}
		} else {
			continue
		}

		var aliases []string
		for _, eid := range peer.Identities {
			if len(eid.EnrollmentID) != 0 {
				aliases = append(aliases, eid.EnrollmentID)
			}
		}

		n.Resolvers = append(n.Resolvers, &Resolver{
			Name: peer.Name,
			Identity: ResolverIdentity{
				ID:      peer.Name,
				MSPType: "bccsp",
				MSPID:   org.MSPID,
				Path:    path,
			},
			Domain:    org.Domain,
			Addresses: addresses,
			Aliases:   aliases,
		})
	}
}

func (n *Network) ViewNodeLocalCertPath(peer *topology.Peer) string {
	if n.topology.NodeOUs {
		switch peer.Role {
		case "":
			return n.PeerUserLocalMSPIdentityCert(peer, peer.Name)
		case "client":
			return n.PeerUserLocalMSPIdentityCert(peer, peer.Name)
		default:
			return n.PeerLocalMSPIdentityCert(peer)
		}
	} else {
		return n.PeerLocalMSPIdentityCert(peer)
	}
}

func (n *Network) ViewNodeLocalPrivateKeyPath(peer *topology.Peer) string {
	if n.topology.NodeOUs {
		switch peer.Role {
		case "":
			return n.PeerUserKey(peer, peer.Name)
		case "client":
			return n.PeerUserKey(peer, peer.Name)
		default:
			return n.PeerKey(peer)
		}
	} else {
		return n.PeerKey(peer)
	}
}
