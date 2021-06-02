/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package nwo

import "gopkg.in/yaml.v2"

type Topology interface {
	Name() string
}

type Topologies struct {
	Topologies []Topology `yaml:"topologies,omitempty"`
}

func (t *Topologies) Export() ([]byte, error) {
	return yaml.Marshal(t)
}
