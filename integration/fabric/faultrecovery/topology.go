/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"slices"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
)

// Orderers names the three raft orderers, so a spec can stop one and keep a quorum.
// The first keeps the default name: nwo deploys chaincode through the orderer named
// "orderer".
var Orderers = []string{"orderer", "orderer2", "orderer3"}

// Topology returns two peers (Org1, Org2), three raft orderers, and the FSC nodes of
// faultrecovery.FSCTopology on Postgres databases with the given prefix.
func Topology(dbPrefix string) []api.Topology {
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespace(views.Namespace, topology.Unanimity("Org1"))
	fabricTopology.Orderers = nil
	for _, name := range Orderers {
		fabricTopology.Orderers = append(fabricTopology.Orderers, &topology.Orderer{Name: name, Organization: "OrdererOrg"})
	}
	for _, p := range fabricTopology.Profiles {
		p.Orderers = slices.Clone(Orderers)
	}

	return []api.Topology{fabricTopology, faultrecovery.FSCTopology(&fabricsdk.SDK{}, dbPrefix)}
}
