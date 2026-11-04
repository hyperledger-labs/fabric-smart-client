/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mixednets

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/twonets/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

// Topology is a Fabric network fab and a Fabric-x network fx, with FSC nodes alice
// and bob on both. Both networks use the same channel name, alice's default network
// is fab and bob's is fx, so each node receives transactions for its non-default
// network of the other platform.
func Topology() []api.Topology {
	fab := fabric.NewTopologyWithName("fab").SetDefault()
	fab.AddOrganizationsByName("Org1", "Org2")
	fab.SetNamespaceApproverOrgs("Org1")
	fab.AddNamespace("ns1", topology.Unanimity("Org1"))

	fx := nwofabricx.NewTopologyWithName("fx")
	fx.AddOrganizationsByName("Org1")
	fx.SetNamespaceApproverOrgs("Org1")
	fx.AddNamespace("ns2", topology.Unanimity("Org1"))

	fscTopology := fsc.NewTopology()
	for node, defaultNetwork := range map[string]string{"alice": "fab", "bob": "fx"} {
		fscTopology.AddNodeByName(node).
			AddOptions(
				fabric.WithNetworkOrganization("fab", "Org1"),
				fabric.WithNetworkOrganization("fx", "Org1"),
				fabric.WithDefaultNetwork(defaultNetwork)).
			RegisterViewFactory("endorse", &views.EndorseFactory{}).
			RegisterViewFactory("status", &views.StatusFactory{}).
			RegisterResponder(&views.Approve{}, &views.Endorse{})
	}
	fscTopology.AddSDK(&SDK{})

	return []api.Topology{fab, fx, fscTopology}
}
