/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package tickets is a ticket booking system on Chaguar: a promoter creates
// events, and resellers in different regions sell seats of the same events.
// Each FSC node serves a REST API and a web UI. See README.md.
package tickets

import (
	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

// Resellers are the FSC nodes that sell seats, one per region.
var Resellers = []string{"reseller-eu", "reseller-us"}

// Topology returns a Chaguar network with three organizations: the promoter,
// the only endorser of the tickets namespace, and one per reseller.
func Topology(sdk node.SDK, commType fsc.P2PCommunicationType) []api.Topology {
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Promoter", "ResellerEU", "ResellerUS")
	fabricTopology.SetNamespaceApproverOrgs("Promoter")
	// Every transaction in the namespace needs the promoter's endorsement.
	fabricTopology.AddNamespace(views.Namespace, topology.Unanimity("Promoter"))

	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:info", "")
	// Each node serves the demo's REST API and UI on its web listener.
	fscTopology.WebEnabled = true

	fscTopology.AddNodeByName(views.PromoterNode).
		AddOptions(fabric.WithOrganization("Promoter")).
		RegisterResponder(&views.SaleResponderView{}, &views.BuyView{})
	for i, name := range Resellers {
		fscTopology.AddNodeByName(name).
			AddOptions(fabric.WithOrganization([]string{"ResellerEU", "ResellerUS"}[i]))
	}

	fscTopology.AddSDK(sdk)
	return []api.Topology{fabricTopology, fscTopology}
}
