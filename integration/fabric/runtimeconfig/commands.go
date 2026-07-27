/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runtimeconfig

import (
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/runtimeconfig/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

// InjectAll renders the full Fabric extension configuration for every FSC peer of the Fabric
// network under test, and injects it at runtime into the corresponding, already-running FSC
// node via the "inject" view. It must be called once, after the infrastructure has started and
// before any IOU view is invoked, on a topology built with Topology (which enables
// Topology.MinimalFSCFabricConfig and registers the "inject" view factory on every FSC node).
func InjectAll(ii *integration.Infrastructure) {
	fabricPlatform := ii.NWOCtx.PlatformsByType(fabric.TopologyName)[0].(*fabric.Platform)
	n := fabricPlatform.Network
	networkName := n.Topology().Name()

	for _, p := range n.Peers {
		if p.Type != topology.FSCPeer {
			continue
		}

		raw, err := n.RenderFSCFabricExtension(p)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		_, err = ii.Client(p.Name).CallView("inject", common.JSONMarshall(&views.InjectNetwork{
			Raw:     []byte(raw),
			Network: networkName,
		}))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
}
