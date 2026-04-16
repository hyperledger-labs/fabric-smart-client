/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"

	dto "github.com/prometheus/client_model/go"
	"github.com/tedsuo/ifrit/grouper"
	"gopkg.in/yaml.v2"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	client2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
)

type (
	PortName string
	Ports    map[PortName]uint16
)

type (
	ExtensionName string
	Extensions    map[ExtensionName][]string
)

const (
	FabricExtension ExtensionName = "FabricExtension"
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

	PortsByPeerID(prefix, id string) Ports
	SetPortsByPeerID(prefix, id string, ports Ports)
	HostByPeerID(prefix, id string) string
	SetHostByPeerID(prefix, id, host string)

	PortsByOrdererID(prefix, id string) Ports
	SetPortsByOrdererID(prefix, id string, ports Ports)
	HostByOrdererID(prefix, id string) string
	SetHostByOrdererID(prefix, id, host string)

	AddIdentityAlias(name, alias string)
	TopologyByName(name string) Topology
	SetConnectionConfig(name string, cc *grpc.ConnectionConfig)
	SetClientSigningIdentity(name string, id client2.SigningIdentity)
	SetAdminSigningIdentity(name string, id client2.SigningIdentity)
	SetViewIdentity(name string, cert []byte)
	ConnectionConfig(name string) *grpc.ConnectionConfig
	ClientSigningIdentity(name string) client2.SigningIdentity
	SetViewClient(name string, c GRPCClient)
	SetWebClient(name string, c WebClient)
	SetCLI(name string, client ViewClient)
	GetViewIdentityAliases(name string) []string
	AdminSigningIdentity(name string) client2.SigningIdentity
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
	StreamCallView(fid string, input []byte) (*client2.Stream, error)
}

type WebClient interface {
	ViewClient
	Metrics() (map[string]*dto.MetricFamily, error)
	StreamCallView(fid string, input []byte) (*client.WSStream, error)
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
