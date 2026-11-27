/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chat

import (
	"maps"
	"slices"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

// Users are the chat users and the organization of each.
var Users = map[string]string{"alice": "org1", "bob": "org1", "charlie": "org2", "dave": "org2"}

const pkg = "github.com/hyperledger-labs/fabric-smart-client/examples/chat"

// Topology returns the chat network: the server in org1 and a client node per user.
func Topology(commType fsc.P2PCommunicationType) []api.Topology {
	topology := fsc.NewTopology()
	topology.P2PCommunicationType = commType
	topology.AddNodeByName(ServerName).
		SetExecutable(pkg + "/cmd/server").
		AddOptions(fsc.WithOrganization("org1"))
	for _, name := range slices.Sorted(maps.Keys(Users)) {
		topology.AddNodeByName(name).
			SetExecutable(pkg + "/cmd/client").
			AddOptions(fsc.WithOrganization(Users[name]))
	}
	return []api.Topology{topology}
}
