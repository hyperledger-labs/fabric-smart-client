/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	. "github.com/onsi/gomega"
)

// NewReplicatedFSCNode
func NewReplicatedFSCNode(fscTopology *fsc.Topology, template *node.Node, numReplicas uint) *Topology {
	Expect(numReplicas).ToNot(BeZero(), "numReplicas must be greater than 0")

	orionTopologyName := template.Name + "-orion-backend"
	orionTopology := NewTopology()
	orionTopology.SetName(orionTopologyName)
	orionTopology.AddDB("backend", template.Name)

	template.AddOptions(
		fsc.WithOrionPersistence(orionTopologyName, "backend", template.Name),
		WithRole(template.Name),
	)

	var nodes []*node.Node
	for i := 0; i < int(numReplicas); i++ {
		n := fscTopology.AddNodeFromTemplate(
			//fmt.Sprintf("%s-%d", template.Name, i),
			template.Name,
			template,
		)
		if i != 0 {
			// disable delivery for all node but the first
			n.AddOptions(fabric.WithDeliveryDisabled())
		}
		nodes = append(nodes, n)
	}
	orionTopology.SetDefaultSDKOnNodes(nodes...)

	return orionTopology
}
