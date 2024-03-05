/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology() []api.Topology {
	// Create an empty FSC topology
	topology := fsc.NewTopology()

	topology.AddNodeByName("alice").RegisterViewFactory("init", &InitiatorViewFactory{})

	topology.AddNodeByName("bob").RegisterResponder(
		&Responder{}, &Initiator{},
	).AddOptions(fsc.WithAlias("bob_alias"), fsc.WithReplicationFactor(2))
	return []api.Topology{topology}
}
