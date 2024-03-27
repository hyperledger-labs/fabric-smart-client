/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology(commType fsc.P2PCommunicationType, replicas map[string]int) []api.Topology {
	if replicas == nil {
		replicas = map[string]int{}
	}
	// Create an empty FSC topology
	topology := fsc.NewTopology()
	topology.P2PCommunicationType = commType

	// Add the initiator fsc node
	topology.AddNodeByName("initiator").
		SetExecutable("github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/cmd/initiator").
		AddOptions(fsc.WithAlias("alice"), fsc.WithReplicationFactor(replicas["initiator"]))
	// Add the responder fsc node
	topology.AddNodeByName("responder").
		SetExecutable("github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/cmd/responder").
		AddOptions(fsc.WithAlias("bob"), fsc.WithReplicationFactor(replicas["responder"]))
	return []api.Topology{topology}
}
