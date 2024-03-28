/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iouorionbe_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iouorionbe"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("EndToEnd", func() {
	Describe("IOU Life Cycle With LibP2P", func() {
		s := NewTestSuite(fsc.LibP2P, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("IOU Life Cycle With Websockets", func() {
		s := NewTestSuite(fsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), true, iouorionbe.Topology(&fabric.SDK{}, commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	iouState := iou.CreateIOU(s.II, "", 10, "approver")
	iou.CheckState(s.II, "borrower", iouState, 10)
	iou.CheckState(s.II, "lender", iouState, 10)
	iou.UpdateIOU(s.II, iouState, 5, "approver")
	iou.CheckState(s.II, "borrower", iouState, 5)
	iou.CheckState(s.II, "lender", iouState, 5)
}
