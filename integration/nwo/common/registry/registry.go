/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package registry

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Client interface {
	CallView(fid string, in []byte) (interface{}, error)
	IsTxFinal(txid string) error
}

type PortName string
type Ports map[PortName]uint16

type ExtensionName string
type Extensions map[ExtensionName]string

const (
	FabricExtension  ExtensionName = "FabricExtension"
	GenericExtension ExtensionName = "GenericExtension"
)

type Builder interface {
	Build(path string) string
}

type SigningIdentity interface {
	Serialize() ([]byte, error)

	Sign(msg []byte) ([]byte, error)
}

type Registry struct {
	NetworkID        string
	Builder          Builder
	PortCounter      uint16
	RootDir          string
	TopologiesByName map[string]nwo.Topology
	PlatformsByName  map[string]nwo.Platform

	PortsByPeerID      map[string]Ports
	ExtensionsByPeerID map[string]Extensions

	ViewClients             map[string]Client
	ViewIdentities          map[string]view.Identity
	ViewIdentityAliases     map[string][]string
	ConnectionConfigs       map[string]*grpc.ConnectionConfig
	ClientSigningIdentities map[string]SigningIdentity
}

func NewRegistry(topologies ...nwo.Topology) *Registry {
	topologiesByName := map[string]nwo.Topology{}
	for _, t := range topologies {
		topologiesByName[t.Name()] = t
	}

	return &Registry{
		ViewClients:             map[string]Client{},
		ViewIdentities:          map[string]view.Identity{},
		ViewIdentityAliases:     map[string][]string{},
		ConnectionConfigs:       map[string]*grpc.ConnectionConfig{},
		ClientSigningIdentities: map[string]SigningIdentity{},
		PortsByPeerID:           map[string]Ports{},
		ExtensionsByPeerID:      map[string]Extensions{},
		TopologiesByName:        topologiesByName,
		PlatformsByName:         map[string]nwo.Platform{},
	}
}

func (reg *Registry) TopologyByName(name string) nwo.Topology {
	return reg.TopologiesByName[name]
}

func (reg *Registry) ReservePort() uint16 {
	reg.PortCounter++
	return reg.PortCounter - 1
}

func (reg *Registry) AddExtension(id string, name ExtensionName, extension string) {
	extensions := reg.ExtensionsByPeerID[id]
	if extensions == nil {
		extensions = Extensions{}
		reg.ExtensionsByPeerID[id] = extensions
	}
	extensions[name] = extension
}

func (reg *Registry) AddIdentityAlias(id string, alias string) {
	reg.ViewIdentityAliases[id] = append(reg.ViewIdentityAliases[id], alias)
}

func (reg *Registry) PlatformByName(name string) nwo.Platform {
	return reg.PlatformsByName[name]
}

func (reg *Registry) AddPlatform(name string, platform nwo.Platform) {
	reg.PlatformsByName[name] = platform
}
