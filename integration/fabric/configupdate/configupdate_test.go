/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configupdate_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/configupdate"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/iou"
)

var _ = Describe("EndToEnd", func() {
	Describe("Channel Config Update With Websockets", Label("T1"), func() {
		s := NewTestSuite(integration.NoReplication, true)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("keeps transacting across channel config updates and a node restart", s.TestSucceeded)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(nodeOpts *integration.ReplicationOptions, tlsEnabled bool) *TestSuite {
	return &TestSuite{TestSuite: integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), integration.WithRaceDetection, configupdate.Topology(&configupdate.Opts{
			ReplicationOpts: nodeOpts,
			TLSEnabled:      tlsEnabled,
		})...)
	})}
}

// allNodes are the FSC nodes in this topology. Every one of them commits the
// CONFIG block independently, so every one of them is asserted on.
var allNodes = []string{"borrower", "lender", "approver1", "approver2"}

// TestSucceeded drives the IOU flow across two channel configuration changes and a node
// restart. Each IOU transaction after a change is the assertion: it needs the FSC nodes to
// still hold a usable view of the channel — its MSPs, for endorsement and validation, and
// its ordering endpoints, for broadcast — which is precisely the state that
// committer.HandleConfig rebuilds when a CONFIG block arrives.
func (s *TestSuite) TestSucceeded() {
	iou.InitApprover(s.II, "approver1")
	iou.InitApprover(s.II, "approver2")

	By("transacting on the channel's initial configuration")
	gomega.Expect(configupdate.BatchTimeout(s.II)).To(gomega.Equal(time.Second),
		"the fixture is expected to start from the configtx default")
	iouState := iou.CreateIOU(s.II, "", 10, "approver1")
	iou.CheckState(s.II, "borrower", iouState, 10)
	iou.CheckState(s.II, "lender", iouState, 10)

	// after the initial IOU flow, before any configuration change
	By("checking every node starts from the genesis configuration")
	configupdate.ExpectConfigSequence(s.II, 0, allNodes...)

	By("changing the channel configuration while the FSC nodes are running")
	configupdate.SetBatchTimeout(s.II, 2*time.Second)
	gomega.Expect(configupdate.BatchTimeout(s.II)).To(gomega.Equal(2 * time.Second))

	// after SetBatchTimeout(2 * time.Second)
	By("checking every node applied the first configuration update")
	configupdate.ExpectConfigSequence(s.II, 1, allNodes...)

	By("transacting again, which requires the nodes to have followed the change")
	iou.UpdateIOU(s.II, iouState, 5, "approver2")
	iou.CheckState(s.II, "borrower", iouState, 5)
	iou.CheckState(s.II, "lender", iouState, 5)

	By("changing the configuration a second time, so more than one update is on the ledger")
	configupdate.SetBatchTimeout(s.II, 500*time.Millisecond)
	gomega.Expect(configupdate.BatchTimeout(s.II)).To(gomega.Equal(500 * time.Millisecond))

	// after SetBatchTimeout(500 * time.Millisecond)
	By("checking every node applied the second configuration update")
	configupdate.ExpectConfigSequence(s.II, 2, allNodes...)

	iou.UpdateIOU(s.II, iouState, 3, "approver1")
	iou.CheckState(s.II, "borrower", iouState, 3)
	iou.CheckState(s.II, "lender", iouState, 3)

	By("restarting a node, so it replays the stored configuration transactions")
	// On start-up a channel calls Committer.ReloadConfigTransactions, which
	// walks the configtx_<n> entries in the node's vault and re-applies each
	// one. With two updates committed above, the borrower's vault holds
	// configtx_0, configtx_1 and configtx_2, so this is the first time in the
	// test suite that the loop runs over more than the genesis entry.
	s.II.StopFSCNode("borrower")
	s.II.StartFSCNode("borrower")

	// after the borrower has been stopped and started again
	By("checking the restarted node replayed both stored configuration transactions")
	configupdate.ExpectConfigSequence(s.II, 2, "borrower")

	By("transacting once more through the restarted node")
	iou.UpdateIOU(s.II, iouState, 1, "approver2")
	iou.CheckState(s.II, "borrower", iouState, 1)
	iou.CheckState(s.II, "lender", iouState, 1)
}
