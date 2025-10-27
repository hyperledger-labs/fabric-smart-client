/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	Describe("IOU Life Cycle With LibP2P", Label("T1"), func() {
		s := NewTestSuite(fsc.LibP2P, integration.NoReplication, true)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("IOU Life Cycle With Websockets", Label("T2"), func() {
		s := NewTestSuite(fsc.WebSocket, integration.NoReplication, true)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("IOU Life Cycle With Websockets and no TLS", Label("T3"), func() {
		s := NewTestSuite(fsc.WebSocket, integration.NoReplication, false)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("IOU Life Cycle With Websockets and replicas", Label("T4"), func() {
		s := NewTestSuite(
			fsc.WebSocket,
			&integration.ReplicationOptions{
				ReplicationFactors: map[string]int{
					"borrower": 3,
					"lender":   2,
				},
				SQLConfigs: map[string]*postgres.ContainerConfig{
					"borrower": postgres.DefaultConfig(postgres.WithDBName("borrower-db")),
					"lender":   postgres.DefaultConfig(postgres.WithDBName("lender-db")),
				},
			},
			true,
		)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceededWithReplicas)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions, tlsEnabled bool) *TestSuite {
	return &TestSuite{TestSuite: integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), integration.WithRaceDetection, iou.Topology(&iou.Opts{
			SDK:             &iou.SDK{},
			CommType:        commType,
			ReplicationOpts: nodeOpts,
			TLSEnabled:      tlsEnabled,
		})...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	iou.InitApprover(s.II, "approver1")
	iou.InitApprover(s.II, "approver2")
	iouState := iou.CreateIOU(s.II, "", 10, "approver1")
	iou.CheckState(s.II, "borrower", iouState, 10)
	iou.CheckState(s.II, "lender", iouState, 10)
	iou.UpdateIOU(s.II, iouState, 5, "approver2")
	iou.CheckState(s.II, "borrower", iouState, 5)
	iou.CheckState(s.II, "lender", iouState, 5)

	iou.CheckLocalMetrics(s.II, "borrower", "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/collectEndorsementsView")
	iou.CheckPrometheusMetrics(s.II, "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/collectEndorsementsView")
	iou.CheckJaegerTraces(s.II, "borrower", "", gomega.And(
		gomega.Not(gomega.BeNil()),
		gomega.Not(gomega.BeEmpty()),
	))
}

func (s *TestSuite) TestSucceededWithReplicas() {
	iou.InitApprover(s.II, "approver1")
	iou.InitApprover(s.II, "approver2")

	iouState := iou.CreateIOUWithBorrower(s.II, "fsc.borrower.0", "", 10, "approver1")
	iou.CheckState(s.II, "fsc.borrower.0", iouState, 10)
	iou.CheckState(s.II, "fsc.borrower.1", iouState, 10)
	iou.CheckState(s.II, "fsc.borrower.2", iouState, 10)
	iou.CheckState(s.II, "fsc.lender.0", iouState, 10)
	iou.CheckState(s.II, "fsc.lender.1", iouState, 10)

	iou.UpdateIOUWithBorrower(s.II, "fsc.borrower.1", iouState, 5, "approver2")
	iou.CheckState(s.II, "fsc.borrower.0", iouState, 5)
	iou.CheckState(s.II, "fsc.borrower.1", iouState, 5)
	iou.CheckState(s.II, "fsc.borrower.2", iouState, 5)
	iou.CheckState(s.II, "fsc.lender.0", iouState, 5)
	iou.CheckState(s.II, "fsc.lender.1", iouState, 5)
}
