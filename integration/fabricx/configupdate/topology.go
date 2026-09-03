/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configupdate

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	configviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/configupdate/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

// Topology returns the fabricx IOU topology, in which every FSC node can additionally
// report the sequence of the channel configuration it holds, through the configseq view.
//
// Nothing else about that topology needs changing for these specs, so it is taken as it
// is: the iou namespace requires unanimous endorsement by Org1, and approver1 is its only
// member, so every IOU call in a spec must go through approver1. approver2 belongs to Org2
// and exists to be asserted on, not to endorse.
func Topology(sdk node.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	topologies := iou.Topology(sdk, commType, replicationOpts)

	for _, t := range topologies {
		fscTopology, ok := t.(*fsc.Topology)
		if !ok {
			continue
		}
		for _, n := range fscTopology.Nodes {
			n.RegisterViewFactory("configseq", &configviews.ConfigSequenceViewFactory{})
		}
	}

	return topologies
}
