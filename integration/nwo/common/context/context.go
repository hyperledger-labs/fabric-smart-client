/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package context

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/identity"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Builder interface {
	Build(path string) string
}

type SigningIdentity interface {
	Serialize() ([]byte, error)

	Sign(msg []byte) ([]byte, error)
}

type Context struct {
	Builder          Builder
	PortCounter      uint16
	rootDir          string
	TopologiesByName map[string]api.Topology
	PlatformsByName  map[string]api.Platform

	portsByPeerID      map[string]api.Ports
	extensionsByPeerID map[string]api.Extensions

	ViewClients             map[string]api.ViewClient
	ViewIdentities          map[string]view.Identity
	ViewIdentityAliases     map[string][]string
	ConnectionConfigs       map[string]*grpc.ConnectionConfig
	ClientSigningIdentities map[string]SigningIdentity
	AdminSigningIdentities  map[string]SigningIdentity
}

func New(rootDir string, portCounter uint16, builder api.Builder, topologies ...api.Topology) *Context {
	topologiesByName := map[string]api.Topology{}
	for _, t := range topologies {
		topologiesByName[t.Type()] = t
	}

	return &Context{
		rootDir:                 rootDir,
		PortCounter:             portCounter,
		Builder:                 builder,
		ViewClients:             map[string]api.ViewClient{},
		ViewIdentities:          map[string]view.Identity{},
		ViewIdentityAliases:     map[string][]string{},
		ConnectionConfigs:       map[string]*grpc.ConnectionConfig{},
		ClientSigningIdentities: map[string]SigningIdentity{},
		AdminSigningIdentities:  map[string]SigningIdentity{},
		portsByPeerID:           map[string]api.Ports{},
		extensionsByPeerID:      map[string]api.Extensions{},
		TopologiesByName:        topologiesByName,
		PlatformsByName:         map[string]api.Platform{},
	}
}

func (c *Context) RootDir() string {
	return c.rootDir
}

func (c *Context) PortsByPeerID(prefix string, id string) api.Ports {
	return c.portsByPeerID[prefix+id]
}

func (c *Context) SetPortsByPeerID(prefix string, id string, ports api.Ports) {
	c.portsByPeerID[prefix+id] = ports
}

func (c *Context) TopologyByName(name string) api.Topology {
	return c.TopologiesByName[name]
}

func (c *Context) ReservePort() uint16 {
	c.PortCounter++
	return c.PortCounter - 1
}

func (c *Context) SetConnectionConfig(name string, cc *grpc.ConnectionConfig) {
	c.ConnectionConfigs[name] = cc
}

func (c *Context) SetClientSigningIdentity(name string, id identity.SigningIdentity) {
	c.ClientSigningIdentities[name] = id
}

func (c *Context) SetAdminSigningIdentity(name string, id identity.SigningIdentity) {
	c.AdminSigningIdentities[name] = id
}

func (c *Context) SetViewIdentity(name string, cert []byte) {
	c.ViewIdentities[name] = cert
}

func (c *Context) ConnectionConfig(name string) *grpc.ConnectionConfig {
	return c.ConnectionConfigs[name]
}

func (c *Context) ClientSigningIdentity(name string) view2.SigningIdentity {
	return c.ClientSigningIdentities[name]
}

func (c *Context) SetViewClient(name string, client api.ViewClient) {
	c.ViewClients[name] = client
}

func (c *Context) GetViewIdentityAliases(name string) []string {
	return c.ViewIdentityAliases[name]
}

func (c *Context) AdminSigningIdentity(name string) view2.SigningIdentity {
	return c.AdminSigningIdentities[name]
}

func (c *Context) ExtensionsByPeerID(name string) api.Extensions {
	return c.extensionsByPeerID[name]
}

func (c *Context) AddExtension(id string, name api.ExtensionName, extension string) {
	extensions := c.extensionsByPeerID[id]
	if extensions == nil {
		extensions = api.Extensions{}
		c.extensionsByPeerID[id] = extensions
	}
	extensions[name] = append(extensions[name], extension)
}

func (c *Context) AddIdentityAlias(id string, alias string) {
	c.ViewIdentityAliases[id] = append(c.ViewIdentityAliases[id], alias)
}

func (c *Context) PlatformByName(name string) api.Platform {
	return c.PlatformsByName[name]
}

func (c *Context) AddPlatform(platform api.Platform) {
	c.PlatformsByName[platform.Name()] = platform
}
