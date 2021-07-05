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

type Relay struct {
	Name     string
	Hostname string
	Port     uint16 `yaml:"port,omitempty"`
	Networks []*Network
	Drivers  []*Driver
}

func (w *Relay) AddFabricNetwork(ft *topology.Topology) *Relay {
	w.Networks = append(w.Networks, &Network{
		Type: "Fabric",
		Name: "Fabric_" + ft.Name(),
	})
	return w
}

type Topology struct {
	TopologyName string `yaml:"name,omitempty"`
	TopologyType string `yaml:"type,omitempty"`
	Relays       []*Relay
}

func NewTopology() *Topology {
	return &Topology{
		TopologyName: TopologyName,
		TopologyType: TopologyName,
		Relays:       []*Relay{},
	}
}

func (t *Topology) Name() string {
	return t.TopologyName
}

func (t *Topology) Type() string {
	return t.TopologyType
}

func (t *Topology) AddRelay(ft *topology.Topology) *Relay {
	r := &Relay{
		Name:     "Fabric_" + ft.Name(),
		Hostname: "localhost",
		Drivers: []*Driver{
			{
				Name:     "Fabric",
				Hostname: "localhost",
			},
		},
	}
	t.Relays = append(t.Relays, r)
	return r
}
