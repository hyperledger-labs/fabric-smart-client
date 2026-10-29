//go:build pkcs11

/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iouhsm_test

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iouhsm"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("EndToEnd", func() {
	// websocket only: identical p2p path to fabric/iou, which keeps libp2p. The
	// variable under test here is HSM signing, not the transport.
	Describe("IOU (With HSM) Life Cycle", Label(fsc.WebSocket), func() {
		s := NewTestSuite(fsc.WebSocket)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("creates and updates IOUs signed with HSM keys", s.TestSucceeded)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), integration.WithRaceDetection, iouhsm.Topology(commType)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	iouState := iou.CreateIOU(s.II, "", 10, "approver")
	iou.CheckState(s.II, "borrower", iouState, 10)
	iou.CheckState(s.II, "lender", iouState, 10)
	iou.UpdateIOU(s.II, iouState, 5, "approver")
	iou.CheckState(s.II, "borrower", iouState, 5)
	iou.CheckState(s.II, "lender", iouState, 5)

	iouState = iou.CreateIOU(s.II, "borrower-hsm-2", 10, "approver")
	iou.CheckState(s.II, "borrower", iouState, 10)
	iou.CheckState(s.II, "lender", iouState, 10)
	iou.UpdateIOU(s.II, iouState, 5, "approver")
	iou.CheckState(s.II, "borrower", iouState, 5)
	iou.CheckState(s.II, "lender", iouState, 5)
}
