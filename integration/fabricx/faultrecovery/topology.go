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
// faultrecovery.FSCTopology on Postgres databases with the given prefix.
func Topology(dbPrefix string) []api.Topology {
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespace(views.Namespace, topology.Unanimity("Org1"))

	return []api.Topology{fabricTopology, faultrecovery.FSCTopology(&fabricxsdk.SDK{}, dbPrefix)}
}
