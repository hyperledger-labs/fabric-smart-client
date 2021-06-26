/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import "gopkg.in/yaml.v2"

// Topology represents a topology of a given network type (fabric, fsc, etc...)
type Topology interface {
	// Name returns the name of the type of network this topology refers to
	Name() string
}

type Topologies struct {
	Topologies []Topology `yaml:"topologies,omitempty"`
}

func (t *Topologies) Export() ([]byte, error) {
	return yaml.Marshal(t)
}
