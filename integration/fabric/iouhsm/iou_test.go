/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iouhsm_test

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iouhsm"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	var (
		ii *integration.Infrastructure
	)

	AfterEach(func() {
		// Stop the ii
		ii.Stop()
	})

	Describe("IOU (With HSM) Life Cycle", func() {
		BeforeEach(func() {
			var err error
			// Create the integration ii
			ii, err = integration.GenerateAt(StartPort(), "", true, iouhsm.Topology(&fabric.SDK{})...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			// Sleep for a while to allow the networks to be ready
			time.Sleep(20 * time.Second)
		})

		It("succeeded", func() {
			iouState := iou.CreateIOU(ii, "", 10, "approver")
			iou.CheckState(ii, "borrower", iouState, 10)
			iou.CheckState(ii, "lender", iouState, 10)
			iou.UpdateIOU(ii, iouState, 5, "approver")
			iou.CheckState(ii, "borrower", iouState, 5)
			iou.CheckState(ii, "lender", iouState, 5)

			iouState = iou.CreateIOU(ii, "borrower-hsm-2", 10, "approver")
			iou.CheckState(ii, "borrower", iouState, 10)
			iou.CheckState(ii, "lender", iouState, 10)
			iou.UpdateIOU(ii, iouState, 5, "approver")
			iou.CheckState(ii, "borrower", iouState, 5)
			iou.CheckState(ii, "lender", iouState, 5)
		})
	})
})
