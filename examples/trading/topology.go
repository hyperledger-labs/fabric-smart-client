/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package trading is a small market on Chaguar where producers negotiate and
// trade goods, written as FSC views. See README.md for a walkthrough.
package trading

import (
	"github.com/hyperledger-labs/fabric-smart-client/examples/trading/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

// Topology returns a Chaguar network where the market operator (Org1) is the
// only endorser of the trading namespace, and one FSC node per producer (Org2).
func Topology(sdk node.SDK, commType fsc.P2PCommunicationType) []api.Topology {
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	// Every trading transaction needs the market's endorsement, and only it.
	fabricTopology.AddNamespace(views.Namespace, topology.Unanimity("Org1"))

	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:info", "")

	// The market sets up the producers and checks every transaction.
	fscTopology.AddNodeByName("market").
		AddOptions(fabric.WithOrganization("Org1")).
		RegisterViewFactory("setup", &views.SetupViewFactory{}).
		RegisterResponder(&views.MarketView{}, &views.ProduceView{}).
		RegisterResponder(&views.MarketView{}, &views.RefineView{}).
		RegisterResponder(&views.MarketView{}, &views.ListView{}).
		RegisterResponder(&views.BoardView{}, &views.BrowseView{}).
		RegisterResponder(&views.MarketView{}, &views.BuyView{})

	// Every producer runs the same views; the market's registry decides what
	// each one may produce or refine.
	for _, name := range []string{"mine", "forest", "refinery", "mill"} {
		fscTopology.AddNodeByName(name).
			AddOptions(fabric.WithOrganization("Org2")).
			RegisterViewFactory("whoami", &views.WhoAmIViewFactory{}).
			RegisterViewFactory("produce", &views.ProduceViewFactory{}).
			RegisterViewFactory("refine", &views.RefineViewFactory{}).
			RegisterViewFactory("list", &views.ListViewFactory{}).
			RegisterViewFactory("browse", &views.BrowseViewFactory{}).
			RegisterViewFactory("buy", &views.BuyViewFactory{}).
			RegisterResponder(&views.SellerView{}, &views.BuyView{}).
			RegisterViewFactory("state", &views.StateViewFactory{})
	}

	fscTopology.AddSDK(sdk)

	return []api.Topology{fabricTopology, fscTopology}
}
