/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package monitoring

type Topology struct {
	TopologyName            string `yaml:"name,omitempty"`
	TopologyType            string `yaml:"type,omitempty"`
	HyperledgerExplorer     bool   `yaml:"hyperledger-explorer,omitempty"`
	HyperledgerExplorerPort int    `yaml:"hyperledger-explorer-port,omitempty"`
	PrometheusGrafana       bool   `yaml:"prometheus-grafana,omitempty"`
	PrometheusPort          int    `yaml:"prometheus-port,omitempty"`
	GrafanaPort             int    `yaml:"grafana-port,omitempty"`
	OPTL                    bool   `yaml:"optl,omitempty"`
	OPTLPort                int    `yaml:"optl-port,omitempty"`
}

func NewTopology() *Topology {
	return &Topology{
		TopologyName:            TopologyName,
		TopologyType:            TopologyName,
		HyperledgerExplorerPort: 8080,
		PrometheusPort:          9090,
		GrafanaPort:             3000,
		OPTLPort:                4319,
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

func (t *Topology) EnableOPTL() {
	t.OPTL = true
}

func (t *Topology) SetOPTLPort(port int) {
	t.OPTLPort = port
}
