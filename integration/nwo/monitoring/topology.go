/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package monitoring

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/monitoring"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/otlp"
)

type Topology struct {
	TopologyName            string `yaml:"name,omitempty"`
	TopologyType            string `yaml:"type,omitempty"`
	HyperledgerExplorer     bool   `yaml:"hyperledger-explorer,omitempty"`
	HyperledgerExplorerPort int    `yaml:"hyperledger-explorer-port,omitempty"`
	PrometheusGrafana       bool   `yaml:"prometheus-grafana,omitempty"`
	PrometheusPort          int    `yaml:"prometheus-port,omitempty"`
	GrafanaPort             int    `yaml:"grafana-port,omitempty"`
	OTLP                    bool   `yaml:"otlp,omitempty"`
	OTLPPort                int    `yaml:"otlp-port,omitempty"`
	JaegerQueryPort         int    `yaml:"jaeger-query-port"`
}

func NewTopology() *Topology {
	return &Topology{
		TopologyName:            TopologyName,
		TopologyType:            TopologyName,
		HyperledgerExplorerPort: 8080,
		PrometheusPort:          monitoring.PrometheusPort,
		GrafanaPort:             3000,
		OTLPPort:                4319,
		JaegerQueryPort:         otlp.JaegerQueryPort,
	}
}

func (t *Topology) Name() string {
	return t.TopologyName
}

func (t *Topology) Type() string {
	return t.TopologyType
}

func (t *Topology) EnableHyperledgerExplorer() {
	t.HyperledgerExplorer = true
}

func (t *Topology) SetHyperledgerExplorerPort(port int) {
	t.HyperledgerExplorerPort = port
}

func (t *Topology) EnablePrometheusGrafana() {
	t.PrometheusGrafana = true
}

func (t *Topology) SetPrometheusPort(port int) {
	t.PrometheusPort = port
}

func (t *Topology) SetGrafanaPort(port int) {
	t.GrafanaPort = port
}

func (t *Topology) EnableOTLP() {
	t.OTLP = true
}

func (t *Topology) SetOTLPPort(port int) {
	t.OTLPPort = port
}
