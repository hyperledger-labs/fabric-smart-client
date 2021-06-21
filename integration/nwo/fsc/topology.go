/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fsc

import "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"

const (
	TopologyName = "fsc"
)

type Logging struct {
	Spec   string `yaml:"spec,omitempty"`
	Format string `yaml:"format,omitempty"`
}

type Topology struct {
	TopologyName string       `yaml:"name,omitempty"`
	Nodes        []*node.Node `yaml:"peers,omitempty"`
	GRPCLogging  bool         `yaml:"grpcLogging,omitempty"`
	Logging      *Logging     `yaml:"logging,omitempty"`
}

// NewTopology returns an empty FSC network topology.
func NewTopology() *Topology {
	return &Topology{
		TopologyName: TopologyName,
		Nodes:        []*node.Node{},
		Logging: &Logging{
			Spec:   "grpc=error:debug",
			Format: "'%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'",
		},
	}
}

func (t *Topology) SetLogging(spec, format string) {
	t.Logging = &Logging{
		Spec:   spec,
		Format: format,
	}
}

func (t *Topology) Name() string {
	return t.TopologyName
}

// AddNodeByTemplate adds a new node with the passed name and template
func (t *Topology) AddNodeByTemplate(name string, template *node.Node) *node.Node {
	n := node.NewNodeFromTemplate(name, template)
	return t.addNode(n)
}

// AddNodeByName adds an empty new node with the passed name
func (t *Topology) AddNodeByName(name string) *node.Node {
	n := node.NewNode(name)
	return t.addNode(n)
}

func (t *Topology) addNode(node *node.Node) *node.Node {
	if len(t.Nodes) == 0 {
		node.Bootstrap = true
	}
	t.Nodes = append(t.Nodes, node)
	return node
}
