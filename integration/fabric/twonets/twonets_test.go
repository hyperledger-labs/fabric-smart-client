/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package twonets_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/twonets"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

var _ = Describe("EndToEnd", func() {
	var (
		ii *integration.Infrastructure
	)

	AfterEach(func() {
		// Stop the ii
		ii.Stop()
	})

	Describe("Two Fabric Networks Life Cycle", func() {
		BeforeEach(func() {
			var err error
			// Create the integration ii
			ii, err = integration.Generate(StartPort(), true, twonets.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
		})

		It("succeeded", func() {
			res, err := ii.Client("alice").CallView("ping", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})
	})
})
