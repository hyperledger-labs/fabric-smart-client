/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"github.com/tedsuo/ifrit/grouper"
	"gopkg.in/yaml.v2"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type PortName string
type Ports map[PortName]uint16

type ExtensionName string
type Extensions map[ExtensionName][]string

const (
	FabricExtension  ExtensionName = "FabricExtension"
	GenericExtension ExtensionName = "GenericExtension"
)

// Topology represents a topology of a given network type (fabric, fsc, etc...)
type Topology interface {
	Name() string
	// Type returns the type of network this topology refers to
	Type() string
}

type Topologies struct {
	Topologies []Topology `yaml:"topologies,omitempty"`
}

func (t *Topologies) Export() ([]byte, error) {
	return yaml.Marshal(t)
}

type Context interface {
	RootDir() string
	ReservePort() uint16

	AddExtension(id string, extension ExtensionName, s string)
	ExtensionsByPeerID(name string) Extensions

	PortsByPeerID(prefix string, id string) Ports
	SetPortsByPeerID(prefix string, id string, ports Ports)

	AddIdentityAlias(name string, alias string)
	TopologyByName(name string) Topology
	SetConnectionConfig(name string, cc *grpc.ConnectionConfig)
	SetClientSigningIdentity(name string, id identity.SigningIdentity)
	SetAdminSigningIdentity(name string, id identity.SigningIdentity)
	SetViewIdentity(name string, cert []byte)
	ConnectionConfig(name string) *grpc.ConnectionConfig
	ClientSigningIdentity(name string) view.SigningIdentity
	SetViewClient(name string, c ViewClient)
	GetViewIdentityAliases(name string) []string
	AdminSigningIdentity(name string) view.SigningIdentity
}

type Builder interface {
	Build(path string) string
}

type ViewClient interface {
	CallView(fid string, in []byte) (interface{}, error)
	IsTxFinal(txid string) error
}

type Platform interface {
	Name() string
	Type() string

	GenerateConfigTree()
	GenerateArtifacts()
	Load()

	Members() []grouper.Member
	PostRun()
	Cleanup()
}

type PlatformFactory interface {
	Name() string
	New(registry Context, t Topology, builder Builder) Platform
}
