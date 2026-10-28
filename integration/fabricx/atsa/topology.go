/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package atsa

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/atsa/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

func Topology(sdk node.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	// Create a fabric-x topology with idemix enabled
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.EnableIdemix()
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespace("asset_transfer", topology.Unanimity("Org1"))

	// Create an FSC topology
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:fabricx=debug:info", "")

	// Approver
	fscTopology.AddNodeByName("approver").
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(replicationOpts.For("approver")...).
		RegisterResponder(&views.ApproverView{}, &views.IssueView{}).
		RegisterResponder(&views.ApproverView{}, &views.AgreeToSellView{}).
		RegisterResponder(&views.ApproverView{}, &views.AgreeToBuyView{}).
		RegisterResponder(&views.ApproverView{}, &views.TransferView{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Issuer
	fscTopology.AddNodeByName("issuer").
		AddOptions(fabric.WithOrganization("Org3")).
		AddOptions(replicationOpts.For("issuer")...).
		RegisterViewFactory("issue", &views.IssueViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Alice
	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithOrganization("Org2"), fabric.WithAnonymousIdentity()).
		AddOptions(replicationOpts.For("alice")...).
		RegisterViewFactory("transfer", &views.TransferViewFactory{}).
		RegisterViewFactory("agreeToSell", &views.AgreeToSellViewFactory{}).
		RegisterViewFactory("agreeToBuy", &views.AgreeToBuyViewFactory{}).
		RegisterResponder(&views.AcceptAssetView{}, &views.IssueView{}).
		RegisterResponder(&views.TransferResponderView{}, &views.TransferView{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Bob
	fscTopology.AddNodeByName("bob").
		AddOptions(fabric.WithOrganization("Org2"), fabric.WithAnonymousIdentity()).
		AddOptions(replicationOpts.For("bob")...).
		RegisterViewFactory("transfer", &views.TransferViewFactory{}).
		RegisterViewFactory("agreeToSell", &views.AgreeToSellViewFactory{}).
		RegisterViewFactory("agreeToBuy", &views.AgreeToBuyViewFactory{}).
		RegisterResponder(&views.AcceptAssetView{}, &views.IssueView{}).
		RegisterResponder(&views.TransferResponderView{}, &views.TransferView{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Add app-specific SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{fabricTopology, fscTopology}
}
