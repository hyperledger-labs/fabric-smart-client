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

// Topology returns the Fabric-x committer with two organizations and the FSC nodes of
// faultrecovery.FSCTopology with the given Postgres database prefix.
func Topology(dbPrefix string) []api.Topology {
	return []api.Topology{fabricxTopology(), faultrecovery.FSCTopology(&fabricxsdk.SDK{}, dbPrefix)}
}

// ReplicatedTopology returns the network of Topology with the FSC nodes of
// faultrecovery.ReplicatedFSCTopology.
func ReplicatedTopology(dbPrefix string) []api.Topology {
	return []api.Topology{fabricxTopology(), faultrecovery.ReplicatedFSCTopology(&fabricxsdk.SDK{}, dbPrefix)}
}

// CommitterPerOrgTopology returns the network of Topology with a committer for each
// organization: Org2Committer for Org2, so bob and charlie use it, and the default committer
// for Org1, which alice uses.
func CommitterPerOrgTopology(dbPrefix string) []api.Topology {
	fabricTopology := fabricxTopology()
	fabricTopology.AddCommitter("Org2", Org2Committer)
	return []api.Topology{fabricTopology, faultrecovery.FSCTopology(&fabricxsdk.SDK{}, dbPrefix)}
}

// Org2Committer is the name of Org2's committer in CommitterPerOrgTopology.
const Org2Committer = "SC2"

func fabricxTopology() *nwofabricx.Topology {
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespace(views.Namespace, topology.Unanimity("Org1"))
	return fabricTopology
}
