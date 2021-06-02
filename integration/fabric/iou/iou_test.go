/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	var (
		network *integration.Network
	)

	AfterEach(func() {
		// Stop the network
		network.Stop()
	})

	Describe("IOU Life Cycle", func() {
		BeforeEach(func() {
			var err error
			// Create the integration network
			network, err = integration.GenNetwork(StartPort(), iou.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration network
			network.Start()
		})

		It("succeeded", func() {
			res, err := network.Client("borrower").CallView(
				"create", common.JSONMarshall(&iou.Create{Amount: 10}),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).NotTo(BeNil())
			id := common.JSONUnmarshalString(res)

			res, err = network.Client("borrower").CallView("query", common.JSONMarshall(&iou.Query{LinearID: id}))
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalInt(res)).To(BeEquivalentTo(10))
			res, err = network.Client("lender").CallView("query", common.JSONMarshall(&iou.Query{LinearID: id}))
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalInt(res)).To(BeEquivalentTo(10))

			Expect(id).NotTo(BeNil())
			_, err = network.Client("borrower").CallView(
				"update", common.JSONMarshall(&iou.Update{LinearID: id, Amount: 5}),
			)
			Expect(err).NotTo(HaveOccurred())

			res, err = network.Client("borrower").CallView("query", common.JSONMarshall(&iou.Query{LinearID: id}))
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalInt(res)).To(BeEquivalentTo(5))
			res, err = network.Client("lender").CallView("query", common.JSONMarshall(&iou.Query{LinearID: id}))
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalInt(res)).To(BeEquivalentTo(5))
		})
	})
})
