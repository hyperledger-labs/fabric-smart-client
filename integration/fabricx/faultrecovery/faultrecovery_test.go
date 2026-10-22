/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	fxfaultrecovery "github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/faultrecovery"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("Fault Recovery with Fabric-x", Label(fsc.WebSocket), func() {
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(integration.FabricXFaultRecoveryPort.StartPortForNode(), "", fxfaultrecovery.Topology("frx")...)
		if err != nil {
			return nil, err
		}
		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
		ii.EnableRaceDetector()
		ii.Generate()
		return ii, nil
	})
	BeforeEach(s.Setup)
	AfterEach(s.TearDown)

	It("commits a transaction on a healthy network", func() {
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
	})
})
