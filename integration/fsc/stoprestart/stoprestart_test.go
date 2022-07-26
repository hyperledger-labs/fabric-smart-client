/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/stoprestart"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

var _ = Describe("EndToEnd", func() {

	Describe("Stop and Restart", func() {
		var (
			ii *integration.Infrastructure
		)

		BeforeEach(func() {
			var err error
			// Create the integration ii
			ii, err = integration.Generate(StartPort(), true, stoprestart.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(3 * time.Second)
		})

		AfterEach(func() {
			// Stop the ii
			ii.Stop()
		})

		It("stop and restart successfully", func() {
			res, err := ii.Client("alice").CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

			ii.StopFSCNode("bob")
			time.Sleep(3 * time.Second)
			ii.StartFSCNode("bob")
			time.Sleep(3 * time.Second)

			res, err = ii.Client("alice").CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

	})

})
