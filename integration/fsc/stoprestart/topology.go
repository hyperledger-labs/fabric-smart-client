/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

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

	topology.AddNodeByName("alice").
		RegisterViewFactory("init", &InitiatorViewFactory{}).
		AddOptions(fsc.WithReplicationFactor(replicas["alice"]))

	topology.AddNodeByName("bob").
		RegisterResponder(&Responder{}, &Initiator{}).
		AddOptions(fsc.WithAlias("bob_alias"), fsc.WithReplicationFactor(replicas["bob"]))
	return []api.Topology{topology}
}
