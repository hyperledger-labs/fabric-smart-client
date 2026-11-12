/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsers_test

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/endorsers"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	fabricendorsers "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/endorsers"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

// The specs share one network: each must leave it healthy, see faultrecovery.ShareNetwork.
var _ = Describe("Endorsers with Fabric", Ordered, ContinueOnFailure, Label(fsc.WebSocket), func() {
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(integration.FabricEndorsersPort.StartPortForNode(), integration.WithRaceDetection, fabricendorsers.Topology()...)
	})
	faultrecovery.ShareNetwork(s)
	endorsers.Specs(s)
})
