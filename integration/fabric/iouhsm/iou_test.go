/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iouhsm_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iouhsm"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("EndToEnd", func() {
	Describe("IOU (With HSM) Life Cycle With LibP2P", func() {
		s := NewTestSuite(fsc.LibP2P, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("IOU (With HSM) Life Cycle With Websockets", func() {
		s := NewTestSuite(fsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("IOU (With HSM) Life Cycle With Websockets and replicas", func() {
		s := NewTestSuite(fsc.WebSocket, &integration.ReplicationOptions{
			ReplicationFactors: map[string]int{
				"borrower": 3,
				"lender":   2,
			},
			SQLConfigs: map[string]*postgres.ContainerConfig{
				"borrower": postgres.DefaultConfig(postgres.WithDBName("borrower-db")),
				"lender":   postgres.DefaultConfig(postgres.WithDBName("lender-db")),
			},
		})
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceededWithReplicas)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), integration.WithRaceDetection, iouhsm.Topology(&iou.SDK{}, commType, nodeOpts)...)
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

func (s *TestSuite) TestSucceededWithReplicas() {
	iouState := iou.CreateIOUWithBorrower(s.II, "fsc.borrower.0", "", 10, "approver")
	iou.CheckState(s.II, "fsc.borrower.0", iouState, 10)
	iou.CheckState(s.II, "fsc.borrower.1", iouState, 10)
	iou.CheckState(s.II, "fsc.borrower.2", iouState, 10)
	iou.CheckState(s.II, "fsc.lender.0", iouState, 10)
	iou.CheckState(s.II, "fsc.lender.1", iouState, 10)

	iou.UpdateIOUWithBorrower(s.II, "fsc.borrower.1", iouState, 5, "approver")
	iou.CheckState(s.II, "fsc.borrower.0", iouState, 5)
	iou.CheckState(s.II, "fsc.borrower.1", iouState, 5)
	iou.CheckState(s.II, "fsc.borrower.2", iouState, 5)
	iou.CheckState(s.II, "fsc.lender.0", iouState, 5)
	iou.CheckState(s.II, "fsc.lender.1", iouState, 5)

	iouState = iou.CreateIOUWithBorrower(s.II, "fsc.borrower.0", "borrower-hsm-2", 10, "approver")
	iou.CheckState(s.II, "fsc.borrower.0", iouState, 10)
	iou.CheckState(s.II, "fsc.borrower.1", iouState, 10)
	iou.CheckState(s.II, "fsc.borrower.2", iouState, 10)
	iou.CheckState(s.II, "fsc.lender.0", iouState, 10)
	iou.CheckState(s.II, "fsc.lender.1", iouState, 10)

	iou.UpdateIOUWithBorrower(s.II, "fsc.borrower.1", iouState, 5, "approver")
	iou.CheckState(s.II, "fsc.borrower.0", iouState, 5)
	iou.CheckState(s.II, "fsc.borrower.1", iouState, 5)
	iou.CheckState(s.II, "fsc.borrower.2", iouState, 5)
	iou.CheckState(s.II, "fsc.lender.0", iouState, 5)
	iou.CheckState(s.II, "fsc.lender.1", iouState, 5)
}
