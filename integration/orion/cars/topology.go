/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cars

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/orion"
	"github.com/hyperledger-labs/fabric-smart-client/integration/orion/cars/views"
)

func Topology() []api.Topology {
	// Orion
	orionTopology := orion.NewTopology()
	orionTopology.AddDB("cars", "alice", "bob", "dmv", "dealer")

	// FSC
	fscTopology := fsc.NewTopology()

	fscTopology.AddNodeByName("alice").AddOptions(
		orion.WithRole("alice"),
	)
	fscTopology.AddNodeByName("bob").AddOptions(
		orion.WithRole("bob"),
	)
	fscTopology.AddNodeByName("dmv").AddOptions(
		orion.WithRole("dmv"),
	).RegisterResponder(&views.MintRequestApprovalFlow{}, &views.MintRequestView{})
	fscTopology.AddNodeByName("dealer").AddOptions(
		orion.WithRole("dealer"),
	).RegisterViewFactory("mintRequest", &views.MintRequestViewFactory{})

	orionTopology.SetDefaultSDK(fscTopology)

	return []api.Topology{orionTopology, fscTopology}
}
