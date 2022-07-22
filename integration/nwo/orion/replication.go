/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	. "github.com/onsi/gomega"
)

// NewReplicatedFSCNode creates a new FSC node that is replicated to the given other nodes.
func NewReplicatedFSCNode(fscTopology *fsc.Topology, template *node.Node, numReplicas int) *Topology {
	Expect(numReplicas > 1).To(BeTrue(), "numReplicas must be greater than 1")

	orionTopologyName := template.Name + "-orion-backend"
	orionTopology := NewTopology()
	orionTopology.SetName(orionTopologyName)
	orionTopology.AddDB("backend", template.Name)

	template.AddOptions(
		fsc.WithOrionPersistence(orionTopologyName, "backend", template.Name),
		fabric.WithOrionVaultPersistence(orionTopologyName, "backend", template.Name),
		WithRole(template.Name),
	)

	var nodes []*node.Node
	for i := 0; i < numReplicas; i++ {
		var nodeName string
		if i == 0 {
			nodeName = template.Name
		} else {
			nodeName = fmt.Sprintf("%s-%d", template.Name, i)
		}
		n := fscTopology.AddNodeFromTemplate(
			nodeName,
			template,
		)
		var linkedIdentities []string
		for j := 0; j < numReplicas; j++ {
			if i != j {
				if j == 0 {
					linkedIdentities = append(linkedIdentities, template.Name)
				} else {
					linkedIdentities = append(linkedIdentities, fmt.Sprintf("%s-%d", template.Name, j))
				}
			}
		}
		n.AddOptions(fabric.WithLinkedIdentities(linkedIdentities...))

		nodes = append(nodes, n)
	}
	orionTopology.SetDefaultSDKOnNodes(nodes...)

	return orionTopology
}
