/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

const (
	TopologyName = "orion"
)

type Topology struct {
	TopologyName string `yaml:"name,omitempty"`
	TopologyType string `yaml:"type,omitempty"`
}

func NewTopology() *Topology {
	return &Topology{
		TopologyName: TopologyName,
		TopologyType: TopologyName,
	}
}

func (t *Topology) Name() string {
	return t.TopologyName
}

func (t *Topology) Type() string {
	return t.TopologyType
}
