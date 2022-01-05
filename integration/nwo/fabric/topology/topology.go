/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import (
	"fmt"
	"strconv"
	"strings"
)

// Topology holds the basic information needed to generate
// fabric configuration files.
type Topology struct {
	TopologyName      string              `yaml:"name,omitempty"`
	TopologyType      string              `yaml:"type,omitempty"`
	Default           bool                `yaml:"default,omitempty"`
	Logging           *Logging            `yaml:"logging,omitempty"`
	Organizations     []*Organization     `yaml:"organizations,omitempty"`
	Peers             []*Peer             `yaml:"peers,omitempty"`
	Consortiums       []*Consortium       `yaml:"consortiums,omitempty"`
	SystemChannel     *SystemChannel      `yaml:"system_channel,omitempty"`
	Channels          []*Channel          `yaml:"channels,omitempty"`
	Consensus         *Consensus          `yaml:"consensus,omitempty"`
	Orderers          []*Orderer          `yaml:"orderers,omitempty"`
	Profiles          []*Profile          `yaml:"profiles,omitempty"`
	Templates         *Templates          `yaml:"templates,omitempty"`
	Chaincodes        []*ChannelChaincode `yaml:"chaincodes,omitempty"`
	PvtTxSupport      bool                `yaml:"pvttxsupport,omitempty"`
	PvtTxCCSupport    bool                `yaml:"pvttxccsupport,omitempty"`
	MSPvtTxSupport    bool                `yaml:"mspvttxsupport,omitempty"`
	MSPvtCCSupport    bool                `yaml:"mspvtccsupport,omitempty"`
	FabTokenSupport   bool                `yaml:"fabtokensupport,omitempty"`
	FabTokenCCSupport bool                `yaml:"fabtokenccsupport,omitempty"`
	GRPCLogging       bool                `yaml:"grpcLogging,omitempty"`
	NodeOUs           bool                `yaml:"nodeous,omitempty"`
	FPC               bool                `yaml:"fpc,omitempty"`
	Weaver            bool                `yaml:"weaver,omitempty"`
	LogPeersToFile    bool                `yaml:"logPeersToFile,omitempty"`
	LogOrderersToFile bool                `yaml:"logOrderersToFile,omitempty"`
}

func (c *Topology) Name() string {
	return c.TopologyName
}

func (c *Topology) Type() string {
	return c.TopologyType
}

func (c *Topology) SetDefault() *Topology {
	c.Default = true
	return c
}

func (c *Topology) SetLogging(spec, format string) {
	c.Logging = &Logging{
		Spec:   spec,
		Format: format,
	}
}

func (c *Topology) AddChaincode(cc *ChannelChaincode) {
	// if a chaincode with the same name exists already, replace
	for i, chaincode := range c.Chaincodes {
		if chaincode.Chaincode.Name == cc.Chaincode.Name {
			// replace
			c.Chaincodes[i] = cc
			return
		}
	}

	c.Chaincodes = append(c.Chaincodes, cc)
}

func (c *Topology) AppendPeer(peer *Peer) {
	c.Peers = append(c.Peers, peer)
}

func (c *Topology) AppendOrganization(org *Organization) {
	c.Organizations = append(c.Organizations, org)
	c.Consortiums[0].Organizations = append(c.Consortiums[0].Organizations, org.Name)
	c.Profiles[1].Organizations = append(c.Profiles[1].Organizations, org.Name)
}

func (c *Topology) EnableNodeOUs() {
	c.NodeOUs = true
	for _, organization := range c.Organizations {
		organization.EnableNodeOUs = c.NodeOUs
	}
}

func (c *Topology) EnableGRPCLogging() {
	c.GRPCLogging = true
}

func (c *Topology) AddOrganization(name string) *fscOrg {
	o := &Organization{
		ID:            name,
		Name:          name,
		MSPID:         name + "MSP",
		Domain:        strings.ToLower(name) + ".example.com",
		EnableNodeOUs: c.NodeOUs,
		Users:         1,
		CA:            &CA{Hostname: "ca"},
	}
	c.AppendOrganization(o)
	return &fscOrg{c: c, o: o}
}

func (c *Topology) AddOrganizationsByName(names ...string) *Topology {
	for _, name := range names {
		c.AddOrganization(name).AddPeer(fmt.Sprintf("%s_peer_0", name))
	}
	return c
}

func (c *Topology) AddOrganizations(num int) *Topology {
	for i := 0; i < num; i++ {
		name := "Org" + strconv.Itoa(i+1)
		c.AddOrganization(name).AddPeer(fmt.Sprintf("%s_peer_0", name))
	}
	return c
}

func (c *Topology) AddOrganizationsByMapping(mapping map[string][]string) *Topology {
	for orgName, peers := range mapping {
		org := c.AddOrganization(orgName)
		for _, peerName := range peers {
			org.AddPeer(peerName)
		}
	}
	return c
}

func (c *Topology) AddPeer(name string, org string, typ PeerType, bootstrap bool, executable string) *Peer {
	peerChannels := []*PeerChannel{}
	for _, channel := range c.Channels {
		peerChannels = append(peerChannels, &PeerChannel{Name: channel.Name, Anchor: true})
	}
	p := &Peer{
		Name:           name,
		Organization:   org,
		Type:           typ,
		Bootstrap:      bootstrap,
		ExecutablePath: executable,
		Channels:       peerChannels,
	}
	c.AppendPeer(p)

	return p
}

func (c *Topology) AddNamespace(name string, policy string, peers ...string) {
	cc := &ChannelChaincode{
		Chaincode: Chaincode{
			Name:            name,
			Version:         "Version-0.0",
			Sequence:        "1",
			InitRequired:    true,
			Path:            "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/chaincode/base",
			Lang:            "golang",
			Label:           name,
			Ctor:            `{"Args":["init"]}`,
			Policy:          policy,
			SignaturePolicy: policy,
		},
		Channel: c.Channels[0].Name,
		Peers:   peers,
	}

	c.AddChaincode(cc)
}

func (c *Topology) AddNamespaceWithUnanimity(name string, orgs ...string) *namespace {
	policy := "AND ("
	for i, org := range orgs {
		if i > 0 {
			policy += ","
		}
		policy += "'" + org + "MSP.member'"
	}
	policy += ")"

	var peers []string
	for _, org := range orgs {
		for _, peer := range c.Peers {
			if peer.Organization == org {
				peers = append(peers, peer.Name)
			}
		}
	}

	cc := &ChannelChaincode{
		Chaincode: Chaincode{
			Name:            name,
			Version:         "Version-0.0",
			Sequence:        "1",
			InitRequired:    true,
			Path:            "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/chaincode/base",
			Lang:            "golang",
			Label:           name,
			Ctor:            `{"Args":["init"]}`,
			Policy:          policy,
			SignaturePolicy: policy,
		},
		Channel: c.Channels[0].Name,
		Peers:   peers,
	}

	c.AddChaincode(cc)

	return &namespace{cc: cc}
}

func (c *Topology) AddNamespaceWithOneOutOfN(name string, orgs ...string) {
	policy := "OutOf (1, "
	for i, org := range orgs {
		if i > 0 {
			policy += ","
		}
		policy += "'" + org + "MSP.member'"
	}
	policy += ")"

	var peers []string
	for _, org := range orgs {
		for _, peer := range c.Peers {
			if peer.Organization == org {
				peers = append(peers, peer.Name)
			}
		}
	}

	cc := &ChannelChaincode{
		Chaincode: Chaincode{
			Name:            name,
			Version:         "Version-0.0",
			Sequence:        "1",
			InitRequired:    true,
			Path:            "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/chaincode/base",
			Lang:            "golang",
			Label:           name,
			Ctor:            `{"Args":["init"]}`,
			Policy:          policy,
			SignaturePolicy: policy,
		},
		Channel: c.Channels[0].Name,
		Peers:   peers,
	}

	c.AddChaincode(cc)
}

func (c *Topology) AddManagedNamespace(name string, policy string, chaincode string, ctor string, peers ...string) {
	InitRequired := true
	if len(ctor) == 0 {
		InitRequired = false
	}

	cc := &ChannelChaincode{
		Chaincode: Chaincode{
			Name:            name,
			Version:         "Version-0.0",
			Sequence:        "1",
			InitRequired:    InitRequired,
			Path:            chaincode,
			Lang:            "golang",
			Label:           name,
			Ctor:            ctor,
			Policy:          policy,
			SignaturePolicy: policy,
		},
		Channel: c.Channels[0].Name,
		Peers:   peers,
	}

	c.AddChaincode(cc)
}

func (c *Topology) SetNamespaceApproverOrgs(orgs ...string) {
	lcePolicy := "AND ("
	for i, org := range orgs {
		if i > 0 {
			lcePolicy += ","
		}
		lcePolicy += "'" + org + "MSP.member'"
	}
	lcePolicy += ")"
	for _, profile := range c.Profiles {
		if profile.Name == "OrgsChannel" {
			for i, policy := range profile.Policies {
				if policy.Name == "LifecycleEndorsement" {
					profile.Policies[i] = &Policy{
						Name: "LifecycleEndorsement",
						Type: "Signature",
						Rule: lcePolicy,
					}
					return
				}
			}
			return
		}
	}
}

func (c *Topology) SetNamespaceApproverOrgsOR(orgs ...string) {
	lcePolicy := "OR ("
	for i, org := range orgs {
		if i > 0 {
			lcePolicy += ","
		}
		lcePolicy += "'" + org + "MSP.member'"
	}
	lcePolicy += ")"
	for _, profile := range c.Profiles {
		if profile.Name == "OrgsChannel" {
			for i, policy := range profile.Policies {
				if policy.Name == "LifecycleEndorsement" {
					profile.Policies[i] = &Policy{
						Name: "LifecycleEndorsement",
						Type: "Signature",
						Rule: lcePolicy,
					}
					return
				}
			}
			return
		}
	}
}

func (c *Topology) EnableLogPeersToFile() {
	c.LogPeersToFile = true
}
