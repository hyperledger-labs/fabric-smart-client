/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package endorsers is the Fabric-x endorsers suite: an app spreads its transactions over
// several endorser nodes and fails over between them; see the shared package
// integration/fabric/common/endorsers.
package endorsers

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/endorsers"
	fviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	fabricxsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
)

// Topology returns Fabric-x with Org1 and Org2, the namespace endorsed by any one member of
// Org2 (endorsers.Policy), and the FSC nodes of endorsers.FSCTopology.
func Topology() []api.Topology {
	fabricxTopology := nwofabricx.NewDefaultTopology()
	fabricxTopology.AddOrganizationsByName("Org1", "Org2")
	// On Fabric-x the approver only deploys the namespace, so it need not be in the policy.
	fabricxTopology.SetNamespaceApproverOrgs("Org1")
	fabricxTopology.AddNamespace(fviews.Namespace, endorsers.Policy)
	return []api.Topology{fabricxTopology, endorsers.FSCTopology(&fabricxsdk.SDK{})}
}
