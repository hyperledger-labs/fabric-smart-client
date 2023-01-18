/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"

func NewEmptyTopology() *topology.Topology {
	return &topology.Topology{
		TopologyName:  "default",
		TopologyType:  "fabric",
		Driver:        "generic",
		TLSEnabled:    true,
		Organizations: []*topology.Organization{},
		Consortiums:   []*topology.Consortium{},
		Consensus:     &topology.Consensus{},
		SystemChannel: &topology.SystemChannel{},
		Orderers:      []*topology.Orderer{},
		Channels:      []*topology.Channel{},
		Profiles:      []*topology.Profile{},
	}
}
