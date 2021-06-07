/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package stoprestart

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology() []nwo.Topology {
	// Create an empty FSC topology
	topology := fsc.NewTopology()

	topology.AddNodeByName("alice").RegisterViewFactory("init", &InitiatorViewFactory{})

	topology.AddNodeByName("bob").RegisterResponder(
		&Responder{}, &Initiator{},
	)
	return []nwo.Topology{topology}
}
