/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package auction is a sealed-bid auction on Chaguar, written as FSC views.
// See README.md for a walkthrough.
package auction

import (
	"github.com/hyperledger-labs/fabric-smart-client/examples/auction/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

// Topology returns a Chaguar network where the auction house (Org1) is the only
// endorser of the auction namespace, and one FSC node per bidder (Org2).
func Topology(sdk node.SDK, commType fsc.P2PCommunicationType) []api.Topology {
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	// Every auction transaction needs the house's endorsement, and only it.
	fabricTopology.AddNamespace(views.Namespace, topology.Unanimity("Org1"))

	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:info", "")

	// The auction house runs the auction and validates every transaction.
	fscTopology.AddNodeByName("house").
		AddOptions(fabric.WithOrganization("Org1")).
		RegisterResponder(&views.BidResponderView{}, &views.PlaceBidView{}).
		RegisterViewFactory("init", &views.InitViewFactory{}).
		RegisterViewFactory("create", &views.CreateAuctionViewFactory{}).
		RegisterViewFactory("close", &views.CloseAuctionViewFactory{}).
		RegisterViewFactory("reveal", &views.RevealViewFactory{}).
		RegisterViewFactory("query", &views.QueryAuctionViewFactory{})

	for _, name := range []string{"alice", "bob", "charlie"} {
		fscTopology.AddNodeByName(name).
			AddOptions(fabric.WithOrganization("Org2")).
			RegisterViewFactory("bid", &views.PlaceBidViewFactory{}).
			RegisterViewFactory("audit", &views.AuditViewFactory{}).
			RegisterViewFactory("query", &views.QueryAuctionViewFactory{})
	}

	fscTopology.AddSDK(sdk)

	return []api.Topology{fabricTopology, fscTopology}
}
