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
	Driver            string              `yaml:"driver,omitempty"`
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
	TLSEnabled        bool                `yaml:"tlsEnabled,omitempty"`
}

func (t *Topology) Name() string {
	return t.TopologyName
}

func (t *Topology) Type() string {
	return t.TopologyType
}

func (t *Topology) SetDefault() *Topology {
	t.Default = true
	return t
}

func (t *Topology) SetLogging(spec, format string) {
	l := &Logging{}
	if len(spec) != 0 {
		l.Spec = spec
	} else {
		l.Spec = t.Logging.Spec
	}
	if len(format) != 0 {
		l.Format = format
	} else {
		l.Format = t.Logging.Format
	}

	t.Logging = l
}

func (t *Topology) AddChaincode(cc *ChannelChaincode) {
	// if a chaincode with the same name exists already, replace
	for i, chaincode := range t.Chaincodes {
		if chaincode.Chaincode.Name == cc.Chaincode.Name {
			// replace
			t.Chaincodes[i] = cc
			return
		}
	}

	t.Chaincodes = append(t.Chaincodes, cc)
}

func (t *Topology) AppendPeer(peer *Peer) {
	t.Peers = append(t.Peers, peer)
}

func (t *Topology) AppendOrganization(org *Organization) {
	t.Organizations = append(t.Organizations, org)
	t.Consortiums[0].Organizations = append(t.Consortiums[0].Organizations, org.Name)
	t.Profiles[1].Organizations = append(t.Profiles[1].Organizations, org.Name)
}

func (t *Topology) EnableNodeOUs() {
	t.NodeOUs = true
	for _, organization := range t.Organizations {
		organization.EnableNodeOUs = t.NodeOUs
	}
}

func (t *Topology) EnableGRPCLogging() {
	t.GRPCLogging = true
}

func (t *Topology) AddOrganization(name string) *fscOrg {
	o := &Organization{
		ID:            name,
		Name:          name,
		MSPID:         name + "MSP",
		MSPType:       "bccsp",
		Domain:        strings.ToLower(name) + ".example.com",
		EnableNodeOUs: t.NodeOUs,
		Users:         1,
		CA:            &CA{Hostname: "ca"},
	}
	t.AppendOrganization(o)
	return &fscOrg{c: t, o: o}
}

func (t *Topology) AddOrganizationsByName(names ...string) *Topology {
	for _, name := range names {
		t.AddOrganization(name).AddPeer(fmt.Sprintf("%s_peer_0", name))
	}
	return t
}

func (t *Topology) AddOrganizations(num int) *Topology {
	for i := 0; i < num; i++ {
		name := "Org" + strconv.Itoa(i+1)
		t.AddOrganization(name).AddPeer(fmt.Sprintf("%s_peer_0", name))
	}
	return t
}

func (t *Topology) AddOrganizationsByMapping(mapping map[string][]string) *Topology {
	for orgName, peers := range mapping {
		org := t.AddOrganization(orgName)
		for _, peerName := range peers {
			org.AddPeer(peerName)
		}
	}
	return t
}

func (t *Topology) AddPeer(name string, org string, typ PeerType, bootstrap bool, executable string) *Peer {
	peerChannels := []*PeerChannel{}
	for _, channel := range t.Channels {
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
	t.AppendPeer(p)

	return p
}

func (t *Topology) AddNamespace(name string, policy string, peers ...string) {
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
		Channel: t.Channels[0].Name,
		Peers:   peers,
	}

	t.AddChaincode(cc)
}

func (t *Topology) AddNamespaceWithUnanimity(name string, orgs ...string) *namespace {
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
		for _, peer := range t.Peers {
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
		Channel: t.Channels[0].Name,
		Peers:   peers,
	}

	t.AddChaincode(cc)

	return &namespace{cc: cc}
}

func (t *Topology) AddNamespaceWithOneOutOfN(name string, orgs ...string) {
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
		for _, peer := range t.Peers {
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
		Channel: t.Channels[0].Name,
		Peers:   peers,
	}

	t.AddChaincode(cc)
}

func (t *Topology) AddManagedNamespace(name string, policy string, chaincode string, ctor string, peers ...string) {
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
		Channel: t.Channels[0].Name,
		Peers:   peers,
	}

	t.AddChaincode(cc)
}

func (t *Topology) SetNamespaceApproverOrgs(orgs ...string) {
	lcePolicy := "AND ("
	for i, org := range orgs {
		if i > 0 {
			lcePolicy += ","
		}
		lcePolicy += "'" + org + "MSP.member'"
	}
	lcePolicy += ")"
	for _, profile := range t.Profiles {
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

func (t *Topology) SetNamespaceApproverOrgsOR(orgs ...string) {
	lcePolicy := "OR ("
	for i, org := range orgs {
		if i > 0 {
			lcePolicy += ","
		}
		lcePolicy += "'" + org + "MSP.member'"
	}
	lcePolicy += ")"
	for _, profile := range t.Profiles {
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

func (t *Topology) EnableLogPeersToFile() {
	t.LogPeersToFile = true
}

func (t *Topology) EnableLogOrderersToFile() {
	t.LogOrderersToFile = true
}
