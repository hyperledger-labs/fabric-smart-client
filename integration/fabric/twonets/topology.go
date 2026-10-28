/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package twonets

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/twonets/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology(commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	// Define two Fabric topologies that differ in what an FSC node must handle per
	// network: alpha runs raft without TLS, beta runs SmartBFT with TLS. Ping commits on
	// both, so the suite covers a node that serves a plaintext and a TLS network at once,
	// and both the CFT and the BFT broadcaster. alpha is alice's default network.
	f1Topology := fabric.NewTopologyWithName("alpha").SetDefault()
	f1Topology.TLSEnabled = false
	f1Topology.AddOrganizationsByName("Org1", "Org2")
	f1Topology.SetNamespaceApproverOrgs("Org1")
	f1Topology.AddNamespace("ns1", topology.Unanimity("Org1"))

	f2Topology := fabric.NewTopologyWithName("beta")
	f2Topology.AddOrganizationsByName("Org3", "Org4")
	f2Topology.SetNamespaceApproverOrgs("Org3")
	f2Topology.AddNamespace("ns2", topology.Unanimity("Org3"))
	// FSC's BFT broadcaster refuses fewer than 4 orderers. One keeps the default name:
	// nwo joins peers and deploys chaincode through the orderer named "orderer".
	betaOrderers := []string{"orderer", "orderer2", "orderer3", "orderer4"}
	f2Topology.Consensus.Type = "BFT"
	f2Topology.Orderers = nil
	for _, name := range betaOrderers {
		f2Topology.Orderers = append(f2Topology.Orderers, &topology.Orderer{Name: name, Organization: "OrdererOrg"})
	}
	for _, p := range f2Topology.Profiles {
		p.Orderers = betaOrderers
		p.ChannelCapabilities = []string{"V3_0"}
	}

	// Define an FSC topology with 2 FSC nodes.
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	// The spec reads alice's orderer connections from /metrics.
	fscTopology.EnablePrometheusMetrics()

	// Add alice's FSC node
	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithNetworkOrganization("alpha", "Org1"),
			fabric.WithNetworkOrganization("beta", "Org3"),
			fabric.WithDefaultNetwork("alpha")).
		AddOptions(replicationOpts.For("alice")...).
		RegisterViewFactory("ping", &views.PingFactory{})

	// Add bob's FSC node
	fscTopology.AddNodeByName("bob").
		AddOptions(
			fabric.WithNetworkOrganization("alpha", "Org1"),
			fabric.WithNetworkOrganization("beta", "Org3"),
			fabric.WithDefaultNetwork("beta")).
		AddOptions(replicationOpts.For("bob")...).
		RegisterResponder(&views.Pong{}, &views.Ping{})

	// Add app-specific SDK to FSC Nodes
	fscTopology.AddSDKForCommType(&SDK{}, commType)

	return []api.Topology{f1Topology, f2Topology, fscTopology}
}
