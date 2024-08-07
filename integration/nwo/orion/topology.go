/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/orion/opts"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
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
	Name  string   `yaml:"name,omitempty"`
	Roles []string `yaml:"roles,omitempty"`
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

func (t *Topology) SetName(name string) {
	t.TopologyName = name
}

func (t *Topology) Name() string {
	return t.TopologyName
}

func (t *Topology) Type() string {
	return t.TopologyType
}

func (t *Topology) SetSDK(fscTopology *fsc.Topology, sdk api.SDK) {
	for _, node := range fscTopology.Nodes {
		node.AddSDK(sdk)
	}
}

func (t *Topology) SetSDKOnNodes(sdk api.SDK, nodes ...*node.Node) {
	for _, node := range nodes {
		node.AddSDK(sdk)
	}
}

func (t *Topology) AddDB(name string, roles ...string) {
	t.DBs = append(t.DBs, DB{
		Name:  name,
		Roles: roles,
	})
}

// Network returns the orion network from the passed context bound to the passed id.
// It returns nil, if nothing is found
func Network(ctx *context.Context, id string) *Platform {
	p := ctx.PlatformByName(id)
	if p == nil {
		return nil
	}
	fp, ok := p.(*Platform)
	if ok {
		return fp
	}
	return nil
}
