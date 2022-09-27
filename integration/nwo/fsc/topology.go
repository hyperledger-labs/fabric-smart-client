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
	TopologyType string       `yaml:"type,omitempty"`
	Nodes        []*node.Node `yaml:"peers,omitempty"`
	GRPCLogging  bool         `yaml:"grpcLogging,omitempty"`
	Logging      *Logging     `yaml:"logging,omitempty"`
	LogToFile    bool         `yaml:"logToFile,omitempty"`

	TraceAggregator string `yaml:"traceAggregator,omitempty"`
	TracingProvider string `yaml:"tracingType,omitempty"`

	MetricsProvider string `yaml:"metricsType,omitempty"`
}

// NewTopology returns an empty FSC network topology.
func NewTopology() *Topology {
	return &Topology{
		TopologyName: TopologyName,
		TopologyType: TopologyName,
		Nodes:        []*node.Node{},
		Logging: &Logging{
			Spec:   "grpc=error:info",
			Format: "'%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'",
		},
		TracingProvider: "none",
		MetricsProvider: "none",
	}
}

func (t *Topology) SetLogging(spec, format string) {
	l := &Logging{}
	if len(spec) != 0 {
		l.Spec = spec
	} else {
		l.Spec = t.Logging.Spec
	}
	if len(format) != 0 {
		l.Format = format
	} else {
		l.Format = t.Logging.Format
	}

	t.Logging = l
}

func (t *Topology) Name() string {
	return t.TopologyName
}

func (t *Topology) Type() string {
	return t.TopologyType
}

// AddNodeFromTemplate adds a new node with the passed name and template
func (t *Topology) AddNodeFromTemplate(name string, template *node.Node) *node.Node {
	n := node.NewNodeFromTemplate(name, template)
	return t.addNode(n)
}

// AddNodeByName adds an empty new node with the passed name
func (t *Topology) AddNodeByName(name string) *node.Node {
	n := node.NewNode(name)
	return t.addNode(n)
}

func (t *Topology) NewTemplate(name string) *node.Node {
	n := node.NewNode(name)
	return n
}

func (t *Topology) EnableUDPTracing() {
	t.TracingProvider = "udp"
	t.TraceAggregator = "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/tracing/server"
}

func (t *Topology) EnableLogToFile() {
	t.LogToFile = true
}

func (t *Topology) EnablePrometheusMetrics() {
	t.MetricsProvider = "prometheus"
}

func (t *Topology) addNode(node *node.Node) *node.Node {
	if len(t.Nodes) == 0 {
		node.Bootstrap = true
	}
	t.Nodes = append(t.Nodes, node)
	return node
}
