/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"os"
	"slices"
	"time"

	"github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

// Orderers names the three raft orderers, so a spec can stop one and keep a quorum.
// The first keeps the default name: nwo deploys chaincode through the orderer named
// "orderer".
var Orderers = []string{"orderer", "orderer2", "orderer3"}

// Topology returns two peers (Org1, Org2), three raft orderers, and the FSC nodes of
// faultrecovery.FSCTopology, running sdk, on Postgres databases with the given prefix.
func Topology(dbPrefix string, sdk node.SDK) []api.Topology {
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

	return []api.Topology{fabricTopology, faultrecovery.FSCTopology(sdk, dbPrefix)}
}

// SetCommitRetries sets committer.retries and committer.retrySleep of every channel in
// the core.yaml of node, so a spec can outlast the commit retry budget without waiting
// for the defaults. Call it after the network is generated and before it starts.
func SetCommitRetries(ii *integration.Infrastructure, node string, retries int, sleep time.Duration) {
	patchCommitter(ii, node, func(committer map[string]any) {
		committer["retries"] = retries
		committer["retrySleep"] = sleep.String()
	})
}

// SetFinalityWaitTimeout sets committer.waitForEventTimeout of every channel in the
// core.yaml of node: how long IsFinal waits for a transaction that is not final yet. Call
// it after the network is generated and before it starts.
func SetFinalityWaitTimeout(ii *integration.Infrastructure, node string, timeout time.Duration) {
	patchCommitter(ii, node, func(committer map[string]any) {
		committer["waitForEventTimeout"] = timeout.String()
	})
}

// patchCommitter applies patch to the committer section of every channel in the core.yaml
// of node.
func patchCommitter(ii *integration.Infrastructure, node string, patch func(committer map[string]any)) {
	patched := 0
	for _, r := range ii.FscPlatform.Peers {
		if r.Name != node {
			continue
		}
		path := ii.FscPlatform.NodeConfigPath(r)
		raw, err := os.ReadFile(path)
		gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred())
		core := map[string]any{}
		gomega.ExpectWithOffset(2, yaml.Unmarshal(raw, &core)).To(gomega.Succeed())

		// fabric holds one map per network, next to scalars such as enabled.
		fabricSection, _ := core["fabric"].(map[string]any)
		for _, network := range fabricSection {
			n, _ := network.(map[string]any)
			channels, _ := n["channels"].([]any)
			for _, ch := range channels {
				c, _ := ch.(map[string]any)
				committer, ok := c["committer"].(map[string]any)
				if !ok {
					continue
				}
				patch(committer)
				patched++
			}
		}

		raw, err = yaml.Marshal(core)
		gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred())
		gomega.ExpectWithOffset(2, os.WriteFile(path, raw, 0o600)).To(gomega.Succeed())
	}
	gomega.ExpectWithOffset(2, patched).To(gomega.BeNumerically(">", 0), "no channel committer configuration found for FSC node [%s]", node)
}
