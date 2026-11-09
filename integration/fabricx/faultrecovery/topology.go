/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"fmt"

	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	fabricnetwork "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	fxnetwork "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	fabricxsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
)

// Topology returns Fabric-x with two organizations, each running a committer (see
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

// CommitterFailoverTopology returns the network of Topology with a second committer in Org2,
// Org2SecondCommitter, so bob and charlie list two committer endpoints. It is kept apart from
// Topology, where stopping Org2Committer must leave bob without a committer.
func CommitterFailoverTopology(dbPrefix string) []api.Topology {
	fabricTopology := fabricxTopology()
	fabricTopology.AddCommitter("Org2")
	return []api.Topology{fabricTopology, faultrecovery.FSCTopology(&fabricxsdk.SDK{}, dbPrefix)}
}

// Org2Committer is the committer of Org2, which bob and charlie use; alice uses Org1's.
var Org2Committer = nwofabricx.CommitterName("Org2")

// Org2SecondCommitter is the committer CommitterFailoverTopology adds to Org2, which AddCommitter
// names after the one Org2 already runs.
const Org2SecondCommitter = "Org2_committer_1"

func fabricxTopology() *nwofabricx.Topology {
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespace(views.Namespace, topology.Unanimity("Org1"))
	return fabricTopology
}

// CommitterEndpoints returns the notification and query service addresses of the committer
// name of org, as an FSC node lists them in its core.yaml.
func CommitterEndpoints(ii *integration.Infrastructure, org, name string) (notification, query string) {
	n := nwofabricx.FxPlatform(ii).Network
	committer := n.Peer(org, name)
	gomega.Expect(committer).NotTo(gomega.BeNil(), "no committer [%s] in [%s]", name, org)
	return fmt.Sprintf("127.0.0.1:%d", n.PeerPort(committer, fabricnetwork.ListenPort)),
		fmt.Sprintf("127.0.0.1:%d", n.PeerPort(committer, fxnetwork.QueryServicePortName))
}
