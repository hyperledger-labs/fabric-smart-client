/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/opts"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	. "github.com/onsi/gomega"
)

// CheckTopology checks the topology of the network
func (n *Network) CheckTopology() {
	if n.Templates == nil {
		n.Templates = &topology.Templates{}
	}
	if n.Logging == nil {
		n.Logging = &topology.Logging{
			Spec:   "debug",
			Format: "'%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'",
		}
	}

	n.CheckTopologyOrderers()
	n.CheckTopologyOrgs(n.CheckTopologyFSCNodes())
	n.CheckTopologyFabricPeers()
	n.CheckTopologyExtensions()
}

// CheckTopologyOrderers checks that the orderers' ports are allocated
func (n *Network) CheckTopologyOrderers() {
	for _, o := range n.Orderers {
		ports := api.Ports{}
		for _, portName := range OrdererPortNames() {
			ports[portName] = n.Context.ReservePort()
		}
		n.Context.SetPortsByOrdererID(n.Prefix, o.ID(), ports)
		n.Context.SetHostByOrdererID(n.Prefix, o.ID(), "0.0.0.0")
	}
}

// CheckTopologyFSCNodes checks that the FSC nodes' are well configured.
// It returns the parameters to be used for cryptogen.
func (n *Network) CheckTopologyFSCNodes() (users map[string]int, userSpecs map[string][]topology.UserSpec) {
	fscTopology := n.Context.TopologyByName("fsc").(*fsc.Topology)

	users = make(map[string]int)
	userSpecs = make(map[string][]topology.UserSpec)
	for _, node := range fscTopology.Nodes {
		var identities []*topology.PeerIdentity

		po := node.PlatformOpts()
		nodeOpts := opts.Get(po)
		orgs := nodeOpts.Organizations()
		if len(orgs) == 0 {
			continue
		}

		org, found := FindOptOrg(orgs, n.topology.TopologyName)
		if !found {
			continue
		}

		if nodeOpts.AnonymousIdentity() {
			identities = append(identities, NewIdemixPeerIdentity("idemix", node.Name))
			n.Context.AddIdentityAlias(node.Name, "idemix")
		}
		for _, label := range nodeOpts.IdemixIdentities() {
			if label == "_default_" {
				continue
			}
			identities = append(identities, NewIdemixPeerIdentity(label, label))
			n.Context.AddIdentityAlias(node.Name, label)
		}
		for _, label := range nodeOpts.X509Identities() {
			if label == "_default_" {
				continue
			}

			identities = append(identities, NewX509PeerIdentity(label, label, "", org, "SW", false))
			users[org.Org] = users[org.Org] + 1
			userSpecs[org.Org] = append(userSpecs[org.Org], topology.UserSpec{Name: label})
			n.Context.AddIdentityAlias(node.Name, label)
		}
		for _, label := range nodeOpts.X509IdentitiesByHSM() {
			if label == "_default_" {
				continue
			}
			identities = append(identities, NewX509PeerIdentity(label, label, "", org, "PKCS11", false))
			users[org.Org] = users[org.Org] + 1
			userSpecs[org.Org] = append(userSpecs[org.Org], topology.UserSpec{Name: label, HSM: true})
			n.Context.AddIdentityAlias(node.Name, label)
		}

		var defaultNetwork bool
		switch {
		case nodeOpts.DefaultNetwork() == "":
			defaultNetwork = n.topology.Default
		case n.topology.TopologyName == nodeOpts.DefaultNetwork():
			defaultNetwork = true
		default:
			defaultNetwork = false
		}
		p := &topology.Peer{
			Name:           node.Name,
			Organization:   org.Org,
			Type:           topology.FSCPeer,
			Role:           nodeOpts.Role(),
			Bootstrap:      node.Bootstrap,
			ExecutablePath: node.ExecutablePath,
			Identities:     identities,
			DefaultNetwork: defaultNetwork,
			FSCNode:        node,
		}
		bccspDefault := "SW"
		if nodeOpts.DefaultIdentityByHSM() {
			bccspDefault = "PKCS11"
		}
		userSpecs[org.Org] = append(userSpecs[org.Org], topology.UserSpec{Name: node.Name, HSM: bccspDefault == "PKCS11"})
		path := ""
		if n.topology.NodeOUs {
			path = n.ViewNodeMSPDir(p)
		}
		defaultIdentityLabel := nodeOpts.DefaultIdentityLabel()
		if len(defaultIdentityLabel) == 0 {
			defaultIdentityLabel = node.Name
		}
		p.Identities = append(p.Identities, NewX509PeerIdentity(defaultIdentityLabel, node.Name, path, org, bccspDefault, true))
		p.DefaultIdentity = defaultIdentityLabel

		n.Peers = append(n.Peers, p)
		n.Context.SetPortsByPeerID("fsc", p.ID(), n.Context.PortsByPeerID(n.Prefix, node.Name))
		n.Context.SetHostByPeerID("fsc", p.ID(), n.Context.HostByPeerID(n.Prefix, node.Name))
	}

	return
}

// CheckTopologyOrgs allocates users for each organization
func (n *Network) CheckTopologyOrgs(users map[string]int, userSpecs map[string][]topology.UserSpec) {
	for _, organization := range n.Organizations {
		organization.Users += users[organization.Name]
		if n.topology.Weaver {
			organization.UserSpecs = append(userSpecs[organization.Name],
				topology.UserSpec{Name: "User1"},
				topology.UserSpec{Name: "User2"},
				topology.UserSpec{Name: "Relay"},
				topology.UserSpec{Name: "RelayAdmin"},
			)
		} else {
			organization.UserSpecs = append(userSpecs[organization.Name],
				topology.UserSpec{Name: "User1"},
				topology.UserSpec{Name: "User2"},
			)
		}
	}
}

// CheckTopologyFabricPeers checks that the fabric peers' ports are allocated
func (n *Network) CheckTopologyFabricPeers() {
	for _, p := range n.Peers {
		if p.Type == topology.FSCPeer {
			continue
		}
		ports := api.Ports{}
		for _, portName := range PeerPortNames() {
			ports[portName] = n.Context.ReservePort()
		}
		n.Context.SetPortsByPeerID(n.Prefix, p.ID(), ports)
		n.Context.SetHostByPeerID(n.Prefix, p.ID(), "0.0.0.0")
	}
}

// CheckTopologyExtensions checks the topology of each extension
func (n *Network) CheckTopologyExtensions() {
	for _, extension := range n.Extensions {
		extension.CheckTopology()
	}
}

// FindOptOrg searches for the organization matching the passed network.
func FindOptOrg(orgs []opts.Organization, network string) (opts.Organization, bool) {
	var org opts.Organization
	found := false
	for _, o := range orgs {
		if o.Network != "" && o.Network == network {
			found = true
			org = o
		}
	}
	if !found {
		for _, o := range orgs {
			if o.Network == "" {
				found = true
				org = o
			}
		}
	}
	return org, found
}

func NewIdemixPeerIdentity(id, eid string) *topology.PeerIdentity {
	return &topology.PeerIdentity{
		ID:           id,
		EnrollmentID: eid,
		MSPType:      "idemix",
		MSPID:        "IdemixOrgMSP",
		Org:          "IdemixOrg",
		Opts:         BCCSPOpts("SW"),
	}
}

func NewX509PeerIdentity(id, eid, path string, org opts.Organization, bccspDefault string, Default bool) *topology.PeerIdentity {
	pid := &topology.PeerIdentity{
		ID:           id,
		EnrollmentID: eid,
		MSPType:      "bccsp",
		MSPID:        org.Org + "MSP",
		Org:          org.Org,
		Opts:         BCCSPOpts(bccspDefault),
	}
	if Default {
		if len(path) != 0 {
			Expect(bccspDefault).To(Equal("SW"))
			pid.Path = path
		}
	}
	return pid
}
