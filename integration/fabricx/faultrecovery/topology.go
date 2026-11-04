/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	fabricxsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
)

// Topology returns Fabric-x with two organizations, each with a committer of its own (see
// Org2Committer), and the FSC nodes of faultrecovery.FSCTopology with the given Postgres
// database prefix.
func Topology(dbPrefix string) []api.Topology {
	return []api.Topology{fabricxTopology(), faultrecovery.FSCTopology(&fabricxsdk.SDK{}, dbPrefix)}
}

// ReplicatedTopology returns the network of Topology with the FSC nodes of
// faultrecovery.ReplicatedFSCTopology.
func ReplicatedTopology(dbPrefix string) []api.Topology {
	return []api.Topology{fabricxTopology(), faultrecovery.ReplicatedFSCTopology(&fabricxsdk.SDK{}, dbPrefix)}
}

// Org2Committer is the committer of Org2, which bob and charlie use; alice uses Org1's.
var Org2Committer = nwofabricx.CommitterName("Org2")

func fabricxTopology() *nwofabricx.Topology {
	fabricTopology := nwofabricx.NewDefaultTopology().EnableCommitterPerOrg()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespace(views.Namespace, topology.Unanimity("Org1"))
	return fabricTopology
}
