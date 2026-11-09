/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mixedversion

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/mixedversion/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fscnode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// Topology returns a Fabric network with Org1 and Org2, and the FSC nodes approver
// (Org1, endorses views.Namespace) and alice (Org2, initiator) on WebSocket. alice has
// two replicas on one Postgres database, so a spec can run them at different releases
// against the same stores.
//
// The nodes of a previous release are built from this topology's generated main, so
// it registers only the views of package views and the Fabric SDK, which exist at the
// previous release.
func Topology() []api.Topology { return newTopology(false) }

// RollingTopology is Topology with the approver on two replicas too, on one Postgres
// database and behind a load balancer (fsc.WithLoadBalancer), so alice's sessions reach
// whichever approver replica serves while the others restart.
func RollingTopology() []api.Topology { return newTopology(true) }

func newTopology(replicatedApprover bool) []api.Topology {
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespace(views.Namespace, topology.Unanimity("Org1"))

	db := &integration.ReplicationOptions{
		ReplicationFactors: map[string]int{"alice": 2},
		SQLConfigs: map[string]*postgres.ContainerConfig{
			"alice": postgres.DefaultConfig(postgres.WithDBName("mixedversion-alice")),
		},
	}
	if replicatedApprover {
		db.ReplicationFactors["approver"] = 2
		db.SQLConfigs["approver"] = postgres.DefaultConfig(postgres.WithDBName("mixedversion-approver"))
		db.NodeOptions = map[string][]fscnode.Option{"approver": {fsc.WithLoadBalancer()}}
	}

	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = fsc.WebSocket
	fscTopology.AddNodeByName("approver").
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(db.For("approver")...).
		RegisterResponder(&views.ApproverView{}, &views.CreateView{}).
		RegisterResponder(&views.ApproverView{}, &views.UpdateView{}).
		RegisterViewFactory("query", &views.QueryViewFactory{})
	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithOrganization("Org2")).
		AddOptions(db.For("alice")...).
		RegisterViewFactory("create", &views.CreateViewFactory{}).
		RegisterViewFactory("update", &views.UpdateViewFactory{}).
		RegisterViewFactory("query", &views.QueryViewFactory{})
	fscTopology.AddSDK(&sdk.SDK{})

	return []api.Topology{fabricTopology, fscTopology}
}
