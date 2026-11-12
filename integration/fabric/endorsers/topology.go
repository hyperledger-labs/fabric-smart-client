/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package endorsers is the Fabric endorsers suite: an app spreads its transactions over
// several endorser nodes and fails over between them; see the shared package
// integration/fabric/common/endorsers.
package endorsers

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/endorsers"
	fviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
)

// Topology returns Fabric with Org1 and Org2, the namespace endorsed by any one member of
// Org2 (endorsers.Policy), and the FSC nodes of endorsers.FSCTopology.
func Topology() []api.Topology {
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	// The namespace is deployed to the peers of the organizations its policy names, so
	// Org2 approves its definition.
	fabricTopology.SetNamespaceApproverOrgs("Org2")
	fabricTopology.AddNamespace(fviews.Namespace, endorsers.Policy)
	return []api.Topology{fabricTopology, endorsers.FSCTopology(&fabricsdk.SDK{})}
}
