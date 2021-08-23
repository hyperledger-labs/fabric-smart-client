/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

const (
	TopologyName = "weaver"
)

type Network struct {
	Type string
	Name string
}

type Driver struct {
	Name     string
	Hostname string
	Port     uint16 `yaml:"port,omitempty"`
}

type InteropChaincode struct {
	Label     string
	Channel   string
	Namespace string
	Path      string
}

type RelayServer struct {
	FabricTopology     *topology.Topology `yaml:"-"`
	FabricTopologyName string
	Name               string
	Hostname           string
	Port               uint16 `yaml:"port,omitempty"`
	Organization       string
	Networks           []*Network
	Drivers            []*Driver
	InteropChaincode   InteropChaincode
}

func (w *RelayServer) AddFabricNetwork(ft *topology.Topology) *RelayServer {
	w.Networks = append(w.Networks, &Network{
		Type: "Fabric",
		Name: ft.Name(),
	})
	return w
}

type Topology struct {
	TopologyName string `yaml:"name,omitempty"`
	TopologyType string `yaml:"type,omitempty"`
	Relays       []*RelayServer
}

func NewTopology() *Topology {
	return &Topology{
		TopologyName: TopologyName,
		TopologyType: TopologyName,
		Relays:       []*RelayServer{},
	}
}

func (t *Topology) Name() string {
	return t.TopologyName
}

func (t *Topology) Type() string {
	return t.TopologyType
}

func (t *Topology) AddRelayServer(ft *topology.Topology, org string) *RelayServer {
	ft.EnableWeaver()
	r := &RelayServer{
		FabricTopology:     ft,
		FabricTopologyName: ft.Name(),
		Name:               ft.Name(),
		Hostname:           "relay-" + ft.Name(),
		Organization:       org,
		Drivers: []*Driver{
			{
				Name:     "Fabric",
				Hostname: "driver-" + ft.Name(),
			},
		},
		InteropChaincode: InteropChaincode{
			Label:     "interop",
			Channel:   ft.Channels[0].Name,
			Namespace: "interop",
			Path:      "github.com/hyperledger-labs/weaver-dlt-interoperability/core/network/fabric-interop-cc/contracts/interop",
		},
	}
	t.Relays = append(t.Relays, r)
	r.AddFabricNetwork(ft)
	return r
}
