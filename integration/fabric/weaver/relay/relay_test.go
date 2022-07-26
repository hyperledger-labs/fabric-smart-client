/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package relay_test

import (
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/weaver/relay"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	var ii *integration.Infrastructure

	AfterEach(func() {
		// Stop the ii
		ii.Stop()
	})

	Describe("Two Fabric Networks with Weaver Relay Life Cycle", func() {

		var testdataPath = "./testdata"

		BeforeEach(func() {
			var err error
			// Create the integration ii
			ii, err = integration.GenerateAt(StartPort(), testdataPath, false, relay.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
		})

		It("succeeded", func() {
			res, err := ii.Client("alice").CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

			// cleanup testdata when test succeeds.
			// if the test fails, we keep the test data for reviewing what went wrong
			err = os.RemoveAll(testdataPath)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
