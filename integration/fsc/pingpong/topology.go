/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology() []api.Topology {
	// Create an empty FSC topology
	topology := fsc.NewTopology()
	topology.SetLogging("debug", "")

	// Add the initiator fsc node
	topology.AddNodeByName("initiator").SetExecutable(
		"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/cmd/initiator",
	).AddOptions(fsc.WithAlias("alice"))
	// Add the responder fsc node
	topology.AddNodeByName("responder").SetExecutable(
		"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/cmd/responder",
	).AddOptions(fsc.WithAlias("bob"))
	return []api.Topology{topology}
}
