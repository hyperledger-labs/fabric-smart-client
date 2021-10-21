/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/orion/opts"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	orion "github.com/hyperledger-labs/fabric-smart-client/platform/orion/sdk"
)

const (
	TopologyName = "orion"
)

func Options(o *node.Options) *opts.Options {
	return opts.Get(o)
}

func WithRole(role string) node.Option {
	return func(o *node.Options) error {
		Options(o).SetRole(role)
		return nil
	}
}

type DB struct {
	Name  string
	Roles []string
}

type Topology struct {
	TopologyName string `yaml:"name,omitempty"`
	TopologyType string `yaml:"type,omitempty"`
	DBs          []DB
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

func (t *Topology) SetDefaultSDK(fscTopology *fsc.Topology) {
	t.SetSDK(fscTopology, &orion.SDK{})
}

func (t *Topology) SetSDK(fscTopology *fsc.Topology, sdk api.SDK) {
	for _, node := range fscTopology.Nodes {
		node.AddSDK(sdk)
	}
}

func (t *Topology) AddDB(name string, dbs ...string) {
	t.DBs = append(t.DBs, DB{
		Name:  name,
		Roles: dbs,
	})
}
