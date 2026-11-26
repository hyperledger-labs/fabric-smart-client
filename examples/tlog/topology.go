/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package tlog is a transparency log run by three orgs on Chaguar, written as
// FSC views.
package tlog

import (
	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

// Topology returns a Chaguar network with one log node per org, org1..org3.
// Every log transaction needs the endorsement of all three orgs.
func Topology(sdk node.SDK, commType fsc.P2PCommunicationType) []api.Topology {
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespace(views.Namespace, topology.Unanimity("Org1", "Org2", "Org3"))

	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:fabricx=info:info", "")

	for _, n := range []struct{ node, org string }{{"org1", "Org1"}, {"org2", "Org2"}, {"org3", "Org3"}} {
		node := fscTopology.AddNodeByName(n.node).
			AddOptions(fabric.WithOrganization(n.org)).
			RegisterViewFactory("init", &views.InitViewFactory{}).
			RegisterViewFactory("log", &views.LogViewFactory{}).
			RegisterResponder(&views.LogResponderView{}, &views.LogView{}).
			RegisterResponder(&views.CosignResponderView{}, &views.CosignView{})
		if n.node == "org3" {
			// org3 is the demo's misbehaving org: its get and checkpoint
			// views lie when the cheat view tells them to.
			node.RegisterViewFactory("cheat", &views.CheatViewFactory{}).
				RegisterViewFactory("get", &views.CheatingGetViewFactory{}).
				RegisterViewFactory("checkpoint", &views.CheatingCheckpointViewFactory{})
			continue
		}
		node.RegisterViewFactory("get", &views.GetViewFactory{}).
			RegisterViewFactory("checkpoint", &views.CheckpointViewFactory{})
	}

	fscTopology.AddSDK(sdk)

	return []api.Topology{fabricTopology, fscTopology}
}
