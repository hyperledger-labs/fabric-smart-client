/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package chaincodetofsc defines the FSC + Fabric-X topology for the
// chaincode-to-fsc tutorial example.
//
// Topology — five FSC nodes across three Fabric organisations:
//
//	+-----------+-----+--------------------------------------------------+
//	| node      | org | role                                             |
//	+-----------+-----+--------------------------------------------------+
//	| issuer    | Org1| initiator: InitLedger, CreateAsset               |
//	| endorser  | Org1| approver: validates every state-changing tx      |
//	|           |     |   (this is the chaincode replacement)            |
//	| auditor   | Org1| observer responder on every state-changing tx    |
//	| alice     | Org2| owner: initiates Update/Delete/Transfer of her   |
//	|           |     |   assets; receiver-acceptance for incoming tx    |
//	| bob       | Org3| owner: same shape as alice                       |
//	+-----------+-----+--------------------------------------------------+
//
// Fabric topology — three orgs, one Fabric-X namespace approved by Org1.
// Endorsement is unanimity within Org1, so the endorser FSC node's
// signature is the only one strictly required at validation time. The
// auditor still signs (as a witness) and the receiver still signs (as
// receiver-acceptance) — those signatures are enforced by the initiator's
// CollectEndorsementsView, not by the namespace policy.
package chaincodetofsc

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/extensions/scv2"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

// FSC node names — exported so tests can address the right node.
const (
	IssuerNode   = "issuer"
	EndorserNode = "endorser"
	AuditorNode  = "auditor"
	AliceNode    = "alice"
	BobNode      = "bob"
)

// Namespace is re-exported from the views package for convenience so test
// code can refer to a single constant.
const Namespace = views.Namespace

// Topology returns the Fabric and FSC topologies for the chaincode-to-fsc
// example. The shape mirrors integration/fabricx/iou/topology.go so that
// reviewers familiar with the IOU example can read it at a glance.
func Topology(sdk node.SDK, commType fsc.P2PCommunicationType) []api.Topology {
	// ── Fabric-X side ──────────────────────────────────────────────────
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespaceWithUnanimity(Namespace, "Org1")

	// ── FSC side ───────────────────────────────────────────────────────
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:fabricx=info:info", "")

	// Endorser — Org1, approver role. Holds the chaincode-equivalent
	// validation logic. Registered as responder for every state-changing
	// initiator view.
	fscTopology.AddNodeByName(EndorserNode).
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(scv2.WithApproverRole()).
		RegisterResponder(&views.EndorserView{}, &views.InitLedgerView{}).
		RegisterResponder(&views.EndorserView{}, &views.CreateAssetView{}).
		RegisterResponder(&views.EndorserView{}, &views.UpdateAssetView{}).
		RegisterResponder(&views.EndorserView{}, &views.DeleteAssetView{}).
		RegisterResponder(&views.EndorserView{}, &views.TransferAssetView{}).
		RegisterViewFactory("init", &views.EndorserInitViewFactory{})

	// Auditor — Org1, witness role. Observes every state change.
	fscTopology.AddNodeByName(AuditorNode).
		AddOptions(fabric.WithOrganization("Org1")).
		RegisterResponder(&views.AuditorView{}, &views.InitLedgerView{}).
		RegisterResponder(&views.AuditorView{}, &views.CreateAssetView{}).
		RegisterResponder(&views.AuditorView{}, &views.UpdateAssetView{}).
		RegisterResponder(&views.AuditorView{}, &views.DeleteAssetView{}).
		RegisterResponder(&views.AuditorView{}, &views.TransferAssetView{})

	// Issuer — Org1, initiator-only. Bootstraps the ledger and creates new
	// assets. Cannot endorse (no approver role).
	fscTopology.AddNodeByName(IssuerNode).
		AddOptions(fabric.WithOrganization("Org1")).
		RegisterViewFactory("init_ledger", &views.InitLedgerViewFactory{}).
		RegisterViewFactory("create_asset", &views.CreateAssetViewFactory{}).
		RegisterViewFactory("read_asset", &views.ReadAssetViewFactory{}).
		RegisterViewFactory("asset_exists", &views.AssetExistsViewFactory{}).
		RegisterViewFactory("get_all_assets", &views.GetAllAssetsViewFactory{})

	// Alice — Org2, owner. Initiates Update/Delete/Transfer of assets she
	// owns, accepts incoming transfers via TransferAssetReceiverView.
	fscTopology.AddNodeByName(AliceNode).
		AddOptions(fabric.WithOrganization("Org2")).
		RegisterResponder(&views.TransferAssetReceiverView{}, &views.TransferAssetView{}).
		RegisterViewFactory("update_asset", &views.UpdateAssetViewFactory{}).
		RegisterViewFactory("delete_asset", &views.DeleteAssetViewFactory{}).
		RegisterViewFactory("transfer_asset", &views.TransferAssetViewFactory{}).
		RegisterViewFactory("read_asset", &views.ReadAssetViewFactory{}).
		RegisterViewFactory("asset_exists", &views.AssetExistsViewFactory{}).
		RegisterViewFactory("get_all_assets", &views.GetAllAssetsViewFactory{})

	// Bob — Org3, owner. Same shape as Alice.
	fscTopology.AddNodeByName(BobNode).
		AddOptions(fabric.WithOrganization("Org3")).
		RegisterResponder(&views.TransferAssetReceiverView{}, &views.TransferAssetView{}).
		RegisterViewFactory("update_asset", &views.UpdateAssetViewFactory{}).
		RegisterViewFactory("delete_asset", &views.DeleteAssetViewFactory{}).
		RegisterViewFactory("transfer_asset", &views.TransferAssetViewFactory{}).
		RegisterViewFactory("read_asset", &views.ReadAssetViewFactory{}).
		RegisterViewFactory("asset_exists", &views.AssetExistsViewFactory{}).
		RegisterViewFactory("get_all_assets", &views.GetAllAssetsViewFactory{})

	fscTopology.AddSDK(sdk)

	return []api.Topology{fabricTopology, fscTopology}
}
