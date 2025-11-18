/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	node2 "github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

const (
	TopologyName = "fsc"
)

type Logging struct {
	Spec         string `yaml:"spec,omitempty"`
	Format       string `yaml:"format,omitempty"`
	OtelSanitize bool   `yaml:"otelSanitize,omitempty"`
}

type P2PCommunicationType = string

const (
	LibP2P    P2PCommunicationType = libp2p.P2PCommunicationType
	WebSocket P2PCommunicationType = rest.P2PCommunicationType
)

type Topology struct {
	TopologyName         string               `yaml:"name,omitempty"`
	TopologyType         string               `yaml:"type,omitempty"`
	Nodes                []*node.Node         `yaml:"peers,omitempty"`
	GRPCLogging          bool                 `yaml:"grpcLogging,omitempty"`
	Logging              *Logging             `yaml:"logging,omitempty"`
	LogToFile            bool                 `yaml:"logToFile,omitempty"`
	Templates            Templates            `yaml:"templates,omitempty"`
	Monitoring           Monitoring           `yaml:"monitoring,omitempty"`
	P2PCommunicationType P2PCommunicationType `yaml:"p2p_communication_type,omitempty"`
	// WebEnabled is used to activate the FSC web server
	WebEnabled bool `yaml:"web_enabled,omitempty"`
}

type Monitoring struct {
	TracingType          tracing.TracerType `yaml:"tracingType,omitempty"`
	TracingEndpoint      string             `yaml:"tracingEndpoint,omitempty"`
	TracingSamplingRatio float64            `yaml:"tracingSamplingRatio,omitempty"`
	MetricsType          string             `yaml:"metricsType,omitempty"`
	TLS                  bool               `yaml:"tls,omitempty"`
}

// NewTopology returns an empty FSC network topology.
func NewTopology() *Topology {
	return &Topology{
		TopologyName: TopologyName,
		TopologyType: TopologyName,
		Nodes:        []*node.Node{},
		Logging: &Logging{
			Spec:         "grpc=error:info",
			Format:       "'%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'",
			OtelSanitize: false,
		},
		Monitoring: Monitoring{
			MetricsType: "none",
			TLS:         true,
		},
		P2PCommunicationType: LibP2P,
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
	l.OtelSanitize = t.Logging.OtelSanitize
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

func (t *Topology) SetBootstrapNode(n *node.Node) {
	for _, n2 := range t.Nodes {
		n2.Bootstrap = false
	}
	n.Bootstrap = true
}

func (t *Topology) ListNodes(ids ...string) []*node.Node {
	if len(ids) == 0 {
		return t.Nodes
	}
	var res []*node.Node
	for _, n := range t.Nodes {
		for _, id := range ids {
			if n.Name == id {
				res = append(res, n)
				break
			}
		}
	}
	return res
}

func (t *Topology) EnableTracing(typ tracing.TracerType) {
	t.EnableTracingWithRatio(typ, 1)
}

func (t *Topology) EnableTracingWithRatio(typ tracing.TracerType, ratio float64) {
	t.Monitoring.TracingType = typ
	t.Monitoring.TracingSamplingRatio = ratio
}

func (t *Topology) EnableLogToFile() {
	t.LogToFile = true
}

func (t *Topology) EnablePrometheusMetrics() {
	t.Monitoring.MetricsType = "prometheus"
	t.WebEnabled = true
}

func (t *Topology) DisablePrometheusTLS() {
	t.Monitoring.TLS = false
}

func (t *Topology) AddSDK(sdk node2.SDK) {
	for _, n := range t.Nodes {
		n.AddSDK(sdk)
	}
}

func (t *Topology) addNode(node *node.Node) *node.Node {
	if len(t.Nodes) == 0 {
		node.Bootstrap = true
	}
	t.Nodes = append(t.Nodes, node)
	return node
}
