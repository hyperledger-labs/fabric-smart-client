/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/web"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	dto "github.com/prometheus/client_model/go"
	"github.com/tedsuo/ifrit/grouper"
	"gopkg.in/yaml.v2"
)

type PortName string
type Ports map[PortName]uint16

type ExtensionName string
type Extensions map[ExtensionName][]string

const (
	FabricExtension ExtensionName = "FabricExtension"
	OrionExtension  ExtensionName = "OrionExtension"
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

	AddPlatform(platform Platform)
	PlatformByName(name string) Platform
	PlatformsByType(typ string) []Platform

	AddExtension(id string, extension ExtensionName, s string)
	ExtensionsByPeerID(name string) Extensions

	PortsByPeerID(prefix string, id string) Ports
	SetPortsByPeerID(prefix string, id string, ports Ports)
	HostByPeerID(prefix string, id string) string
	SetHostByPeerID(prefix string, id string, host string)

	PortsByOrdererID(prefix string, id string) Ports
	SetPortsByOrdererID(prefix string, id string, ports Ports)
	HostByOrdererID(prefix string, id string) string
	SetHostByOrdererID(prefix string, id string, host string)

	AddIdentityAlias(name string, alias string)
	TopologyByName(name string) Topology
	SetConnectionConfig(name string, cc *grpc.ConnectionConfig)
	SetClientSigningIdentity(name string, id view.SigningIdentity)
	SetAdminSigningIdentity(name string, id view.SigningIdentity)
	SetViewIdentity(name string, cert []byte)
	ConnectionConfig(name string) *grpc.ConnectionConfig
	ClientSigningIdentity(name string) view.SigningIdentity
	SetViewClient(name string, c GRPCClient)
	SetWebClient(name string, c WebClient)
	SetCLI(name string, client ViewClient)
	GetViewIdentityAliases(name string) []string
	AdminSigningIdentity(name string) view.SigningIdentity
	IgnoreSigHUP() bool
}

type Builder interface {
	Build(path string) string
}

type ViewClient interface {
	CallViewWithContext(ctx context.Context, fid string, in []byte) (interface{}, error)
	CallView(fid string, in []byte) (interface{}, error)
}

type GRPCClient interface {
	ViewClient
	StreamCallView(fid string, input []byte) (*view.Stream, error)
}

type WebClient interface {
	ViewClient
	Metrics() (map[string]*dto.MetricFamily, error)
	StreamCallView(fid string, input []byte) (*web.WSStream, error)
}

type Platform interface {
	Name() string
	Type() string

	GenerateConfigTree()
	GenerateArtifacts()
	Load()

	Members() []grouper.Member
	PostRun(load bool)
	Cleanup()
}

type PlatformFactory interface {
	Name() string
	New(registry Context, t Topology, builder Builder) Platform
}
