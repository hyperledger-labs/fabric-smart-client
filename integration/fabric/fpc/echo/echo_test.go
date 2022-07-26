/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package echo_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/fpc/echo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/fpc/echo/views"
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

	Describe("Echo FPC", func() {
		BeforeEach(func() {
			var err error
			// Create the integration ii
			ii, err = integration.Generate(StartPort(), true, echo.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
		})

		It("succeeded", func() {
			provisionedEnclavesBoxed, err := ii.Client("alice").CallView(
				"ListProvisionedEnclaves",
				common.JSONMarshall(&views.ListProvisionedEnclaves{
					CID: "echo",
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			var provisionedEnclaves []string
			common.JSONUnmarshal(provisionedEnclavesBoxed.([]byte), &provisionedEnclaves)
			Expect(len(provisionedEnclaves)).To(BeEquivalentTo(1))

			resBoxed, err := ii.Client("alice").CallView(
				"Echo",
				common.JSONMarshall(&views.Echo{
					Function: "myFunction",
					Args:     []string{"arg1", "arg2", "arg3"},
				}),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(resBoxed).To(BeEquivalentTo("myFunction"))
		})
	})
})
