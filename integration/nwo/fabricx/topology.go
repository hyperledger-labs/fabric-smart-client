/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

const DefaultTopologyName = "default"

// NewDefaultTopology is a configuration with two organizations and one peer per org.
func NewDefaultTopology() *topology.Topology {
	return NewTopologyWithName(DefaultTopologyName).SetDefault()
}

// NewTopologyWithName is a configuration with two organizations and one peer per org.
func NewTopologyWithName(name string) *topology.Topology {
	// we use a classic fabric topo as our reference
	topo := fabric.NewTopologyWithName(name)

	topo.SetLogging("grpc=error:info", "")

	// TODO: the ordering service provided by the committer all-in-one does not support TLS;
	// 	once supported we can remove this
	topo.TLSEnabled = false

	// set fabricx specific settings
	topo.TopologyType = PlatformName
	topo.Driver = PlatformName

	return topo
}
