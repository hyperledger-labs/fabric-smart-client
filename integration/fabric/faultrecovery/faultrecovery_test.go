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
	fabricfaultrecovery "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("Fault Recovery with Fabric", Label(fsc.WebSocket), func() {
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(
			integration.FabricFaultRecoveryPort.StartPortForNode(),
			integration.WithRaceDetection,
			fabricfaultrecovery.Topology("fr")...,
		)
	})
	BeforeEach(s.Setup)
	AfterEach(s.TearDown)

	It("commits a transaction on a healthy network", func() {
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
	})

	faultrecovery.CrashSpecs(s)
})
