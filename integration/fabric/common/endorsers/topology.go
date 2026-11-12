/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package endorsers holds what the Fabric and Fabric-x endorsers suites share: the FSC
// nodes, the helpers that drive them, and the specs both suites run. An app spreads its
// transactions over several endorser nodes of one organization, any one of which satisfies
// the namespace's endorsement policy, and fails over between them; see views.EndorseAnyView.
package endorsers

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/endorsers/views"
	fviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

// Endorsers are the endorser nodes of FSCTopology, all in Org2.
var Endorsers = []string{"endorser1", "endorser2", "endorser3"}

// Policy is the endorsement policy of the namespace the transactions write to: any one
// endorser of Org2 satisfies it.
var Policy = topology.OneOutOfN("Org2")

// FSCTopology returns the FSC node alice (Org1), which initiates every transaction, and the
// endorser nodes of Endorsers (Org2), installing sdk on all of them. Each node is separate,
// with its own identity and its own SQLite database, which survives a restart.
func FSCTopology(sdk node.SDK) *fsc.Topology {
	fscTopology := fsc.NewTopology()
	// The leak check of faultrecovery.ShareNetwork reads every node's metrics.
	fscTopology.EnablePrometheusMetrics()
	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithOrganization("Org1"), fabric.WithDefaultSqlitePersistence()).
		RegisterViewFactory("endorseAny", &views.EndorseAnyViewFactory{}).
		RegisterViewFactory("order", &fviews.OrderViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
		RegisterViewFactory("query", &fviews.QueryViewFactory{})
	for _, name := range Endorsers {
		fscTopology.AddNodeByName(name).
			AddOptions(fabric.WithOrganization("Org2"), fabric.WithDefaultSqlitePersistence()).
			RegisterResponder(&fviews.ResponderView{}, &views.EndorseAnyView{}).
			RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
			RegisterViewFactory("query", &fviews.QueryViewFactory{})
	}
	fscTopology.AddSDK(sdk)
	return fscTopology
}
