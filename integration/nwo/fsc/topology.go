/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fsc

const (
	TopologyName = "fsc"
)

type Logging struct {
	Spec   string `yaml:"spec,omitempty"`
	Format string `yaml:"format,omitempty"`
}

type Topology struct {
	TopologyName string   `yaml:"name,omitempty"`
	Nodes        []*Node  `yaml:"peers,omitempty"`
	GRPCLogging  bool     `yaml:"grpcLogging,omitempty"`
	Logging      *Logging `yaml:"logging,omitempty"`
}

func NewTopology() *Topology {
	return &Topology{
		TopologyName: TopologyName,
		Nodes:        []*Node{},
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
func (t *Topology) AddNodeByTemplate(name string, template *Node) *Node {
	n := NewNodeFromTemplate(name, template)
	return t.addNode(n)
}

// AddNodeByName adds a new node with the passed name
func (t *Topology) AddNodeByName(name string) *Node {
	n := NewNode(name)
	return t.addNode(n)
}

func (t *Topology) addNode(node *Node) *Node {
	if len(t.Nodes) == 0 {
		node.Bootstrap = true
	}
	t.Nodes = append(t.Nodes, node)
	return node
}
