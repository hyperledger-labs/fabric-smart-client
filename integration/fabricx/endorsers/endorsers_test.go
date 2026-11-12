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
	fxendorsers "github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/endorsers"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

// The specs share one network: each must leave it healthy, see faultrecovery.ShareNetwork.
var _ = Describe("Endorsers with Fabric-x", Ordered, ContinueOnFailure, Label(fsc.WebSocket), func() {
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(integration.FabricXEndorsersPort.StartPortForNode(), "", fxendorsers.Topology()...)
		if err != nil {
			return nil, err
		}
		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
		ii.EnableRaceDetector()
		ii.Generate()
		return ii, nil
	})
	faultrecovery.ShareNetwork(s)
	endorsers.Specs(s)
})
