/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
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

	Describe("IOU Life Cycle", func() {
		BeforeEach(func() {
			var err error
			// Create the integration ii
			ii, err = integration.GenerateAt(StartPort(), "", true, iou.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			// Sleep for a while to allow the networks to be ready
			time.Sleep(20 * time.Second)
		})

		It("succeeded", func() {
			res, err := ii.Client("borrower").CallView(
				"create", common.JSONMarshall(&views.Create{
					Amount:   10,
					Lender:   ii.Identity("lender"),
					Approver: ii.Identity("approver"),
				}),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			id := common.JSONUnmarshalString(res)

			res, err = ii.CLI("borrower").CallView("query", common.JSONMarshall(&views.Query{LinearID: id}))
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalInt(res)).To(BeEquivalentTo(10))
			res, err = ii.Client("lender").CallView("query", common.JSONMarshall(&views.Query{LinearID: id}))
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalInt(res)).To(BeEquivalentTo(10))

			Expect(id).NotTo(BeNil())
			txIDBoxed, err := ii.Client("borrower").CallView(
				"update", common.JSONMarshall(&views.Update{
					LinearID: id,
					Amount:   5,
					Approver: ii.Identity("approver"),
				}),
			)
			Expect(err).NotTo(HaveOccurred())
			txID := common.JSONUnmarshalString(txIDBoxed)
			Expect(ii.Client("lender").IsTxFinal(txID)).NotTo(HaveOccurred())

			res, err = ii.Client("borrower").CallView("query", common.JSONMarshall(&views.Query{LinearID: id}))
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalInt(res)).To(BeEquivalentTo(5))
			res, err = ii.Client("lender").CallView("query", common.JSONMarshall(&views.Query{LinearID: id}))
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalInt(res)).To(BeEquivalentTo(5))
		})
	})
})
