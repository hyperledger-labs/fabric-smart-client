/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
)

type PeerType string

const (
	FabricPeer PeerType = "FabricPeer"
	FSCPeer    PeerType = "FSCNode"
)

type Logging struct {
	Spec   string `yaml:"spec,omitempty"`
	Format string `yaml:"format,omitempty"`
}

type UserSpec struct {
	Name string `yaml:"Name"`
	HSM  bool   `yaml:"HSM"`
}

// Organization models information about an Organization. It includes
// the information needed to populate an MSP with cryptogen.
type Organization struct {
	ID            string     `yaml:"id,omitempty"`
	MSPID         string     `yaml:"msp_id,omitempty"`
	MSPType       string     `yaml:"msp_type,omitempty"`
	Name          string     `yaml:"name,omitempty"`
	Domain        string     `yaml:"domain,omitempty"`
	EnableNodeOUs bool       `yaml:"enable_node_organizational_units"`
	Users         int        `yaml:"users,omitempty"`
	UserSpecs     []UserSpec `yaml:"Specs,omitempty"`
	CA            *CA        `yaml:"ca,omitempty"`
}

type CA struct {
	Hostname string `yaml:"hostname,omitempty"`
}

// A Consortium is a named collection of Organizations. It is used to populate
// the Orderer geneesis block profile.
type Consortium struct {
	Name          string   `yaml:"name,omitempty"`
	Organizations []string `yaml:"organizations,omitempty"`
}

// Consensus indicates the orderer types (we only support SOLO for testing)
type Consensus struct {
	Type string `yaml:"type,omitempty"`
}

// The SystemChannel declares the name of the network system channel and its
// associated configtxgen profile name.
type SystemChannel struct {
	Name    string `yaml:"name,omitempty"`
	Profile string `yaml:"profile,omitempty"`
}

// Channel associates a channel name with a configtxgen profile name.
type Channel struct {
	Name        string `yaml:"name,omitempty"`
	Profile     string `yaml:"profile,omitempty"`
	BaseProfile string `yaml:"baseprofile,omitempty"`
	Default     bool   `yaml:"default,omitempty"`
}

// Orderer defines an orderer instance and its owning organization.
type Orderer struct {
	Name         string `yaml:"name,omitempty"`
	Organization string `yaml:"organization,omitempty"`
}

// ID provides a unique identifier for an orderer instance.
func (o Orderer) ID() string {
	return fmt.Sprintf("%s.%s", o.Organization, o.Name)
}

type PostRunInvocation struct {
	FunctionName   string
	ExpectedResult interface{}
	Args           [][]byte
}

type ChannelChaincode struct {
	Chaincode          Chaincode           `yaml:"chaincode,omitempty"`
	PrivateChaincode   PrivateChaincode    `yaml:"privatechaincode,omitempty"`
	Path               string              `yaml:"path,omitempty"`
	Channel            string              `yaml:"channel,omitempty"`
	Peers              []string            `yaml:"peers,omitempty"`
	Private            bool                `yaml:"private,omitempty"`
	PostRunInvocations []PostRunInvocation `yaml:"postruninvocations,omitempty"`
}

func (c *ChannelChaincode) AddPostRunInvocation(functionName string, expectedResult interface{}, args ...[]byte) {
	c.PostRunInvocations = append(c.PostRunInvocations, PostRunInvocation{
		FunctionName:   functionName,
		ExpectedResult: expectedResult,
		Args:           args,
	})
}

type Policy struct {
	Name string
	Type string
	Rule string
}

type BCCSP = config.BCCSP

type SoftwareProvider = config.SoftwareProvider

type PKCS11 = config.PKCS11

type KeyIDMapping = config.KeyIDMapping

type PeerIdentity struct {
	ID           string
	Default      bool
	EnrollmentID string
	MSPType      string
	MSPID        string
	CacheSize    int
	Org          string
	Path         string `yaml:"path,omitempty"`
	Opts         *BCCSP `yaml:"opts,omitempty"`
}

// Peer defines a peer instance, it's owning organization, and the list of
// channels that the peer should be joined to.
type Peer struct {
	Name            string          `yaml:"name,omitempty"`
	Organization    string          `yaml:"organization,omitempty"`
	Type            PeerType        `yaml:"type,omitempty"`
	Bootstrap       bool            `yaml:"bootstrap,omitempty"`
	ExecutablePath  string          `yaml:"executablepath,omitempty"`
	Role            string          `yaml:"role,omitempty"`
	Channels        []*PeerChannel  `yaml:"channels,omitempty"`
	DefaultIdentity string          `yaml:"defaultMSP,omitempty"`
	Identities      []*PeerIdentity `yaml:"identities,omitempty"`
	Usage           string          `yaml:"usage,omitempty"`
	SkipInit        bool            `yaml:"skipinit,omitempty"`
	SkipRunning     bool            `yaml:"skiprunning,omitempty"`
	TLSDisabled     bool            `yaml:"tlsdisabled,omitempty"`
	Hostname        string          `yaml:"hostname,omitempty"`
	DevMode         bool
	DefaultNetwork  bool       `yaml:"-"`
	FSCNode         *node.Node `yaml:"-"`
}

// ID provides a unique identifier for a peer instance.
func (p *Peer) ID() string {
	return fmt.Sprintf("%s.%s", p.Organization, p.Name)
}

// Anchor returns true if this peer is an anchor for any channel it has joined.
func (p *Peer) Anchor() bool {
	for _, c := range p.Channels {
		if c.Anchor {
			return true
		}
	}
	return false
}

// PeerChannel names of the channel a peer should be joined to and whether or
// not the peer should be an anchor for the channel.
type PeerChannel struct {
	Name   string `yaml:"name,omitempty"`
	Anchor bool   `yaml:"anchor"`
}

// Profile encapsulates basic information for a configtxgen profile.
type Profile struct {
	Name                string    `yaml:"name,omitempty"`
	Orderers            []string  `yaml:"orderers,omitempty"`
	Consortium          string    `yaml:"consortium,omitempty"`
	Organizations       []string  `yaml:"organizations,omitempty"`
	AppCapabilities     []string  `yaml:"app_capabilities,omitempty"`
	ChannelCapabilities []string  `yaml:"channel_capabilities,omitempty"`
	Policies            []*Policy `yaml:"policies,omitempty"`
}
