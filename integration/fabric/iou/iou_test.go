/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou_test

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	Describe("IOU Life Cycle With LibP2P", func() {
		s := TestSuite{commType: fsc.LibP2P}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("IOU Life Cycle With Websockets", func() {
		s := TestSuite{commType: fsc.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("IOU Life Cycle With Websockets and replicas", func() {
		s := TestSuite{
			commType: fsc.WebSocket,
			replicas: map[string]int{
				"borrower": 3,
				"lender":   2,
			},
			sqlConfigs: map[string]*sql.PostgresConfig{
				"borrower": sql.DefaultConfig("borrower-db"),
				"lender":   sql.DefaultConfig("lender-db"),
			},
		}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceededWithReplicas)
	})
})

type TestSuite struct {
	commType   fsc.P2PCommunicationType
	replicas   map[string]int
	sqlConfigs map[string]*sql.PostgresConfig

	closeFunc func()
	ii        *integration.Infrastructure
}

func (s *TestSuite) TearDown() {
	s.ii.Stop()
	s.closeFunc()
}

func (s *TestSuite) Setup() {
	closeFunc, err := sql.StartPostgresWithFmt(s.sqlConfigs)
	Expect(err).NotTo(HaveOccurred())
	s.closeFunc = closeFunc
	// Create the integration ii
	ii, err := integration.Generate(StartPort(), true, iou.Topology(&fabric.SDK{}, s.commType, s.replicas, s.sqlConfigs)...)
	Expect(err).NotTo(HaveOccurred())
	s.ii = ii
	// Start the integration ii
	ii.Start()
	// Sleep for a while to allow the networks to be ready
	time.Sleep(20 * time.Second)
}

func (s *TestSuite) TestSucceeded() {
	iou.InitApprover(s.ii, "approver1")
	iou.InitApprover(s.ii, "approver2")
	iouState := iou.CreateIOU(s.ii, "", 10, "approver1")
	iou.CheckState(s.ii, "borrower", iouState, 10)
	iou.CheckState(s.ii, "lender", iouState, 10)
	iou.UpdateIOU(s.ii, iouState, 5, "approver2")
	iou.CheckState(s.ii, "borrower", iouState, 5)
	iou.CheckState(s.ii, "lender", iouState, 5)
}

func (s *TestSuite) TestSucceededWithReplicas() {
	iou.InitApprover(s.ii, "approver1")
	iou.InitApprover(s.ii, "approver2")

	iouState := iou.CreateIOUWithBorrower(s.ii, "fsc.borrower.0", "", 10, "approver1")
	iou.CheckState(s.ii, "fsc.borrower.0", iouState, 10)
	iou.CheckState(s.ii, "fsc.borrower.1", iouState, 10)
	iou.CheckState(s.ii, "fsc.borrower.2", iouState, 10)
	iou.CheckState(s.ii, "fsc.lender.0", iouState, 10)
	iou.CheckState(s.ii, "fsc.lender.1", iouState, 10)

	iou.UpdateIOUWithBorrower(s.ii, "fsc.borrower.1", iouState, 5, "approver2")
	iou.CheckState(s.ii, "fsc.borrower.0", iouState, 5)
	iou.CheckState(s.ii, "fsc.borrower.1", iouState, 5)
	iou.CheckState(s.ii, "fsc.borrower.2", iouState, 5)
	time.Sleep(15 * time.Second)
	iou.CheckState(s.ii, "fsc.lender.0", iouState, 5)
	iou.CheckState(s.ii, "fsc.lender.1", iouState, 5)
}
