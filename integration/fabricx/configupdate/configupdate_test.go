/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configupdate_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/configupdate"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/iou"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	fxnetwork "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("EndToEnd", func() {
	Describe("Channel Config Update", Label("T1"), func() {
		s := NewTestSuite(nwofsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("Refused configuration", Label("T2"), func() {
		s := NewTestSuite(nwofsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("keeps the configuration in force", s.TestRefused)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType nwofsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{TestSuite: integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(
			integration.FabricXConfigUpdatePort.StartPortForNode(), "",
			configupdate.Topology(&iou.SDK{}, commType, nodeOpts)...,
		)
		if err != nil {
			return nil, err
		}

		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
		ii.Generate()

		return ii, nil
	})}
}

// expectConfigSequence waits for every named node to report the given channel
// configuration sequence. A configuration reaches a node asynchronously, through the
// config monitor's next poll, so this polls rather than asserting once.
func expectConfigSequence(s *TestSuite, expected int, nodes ...string) {
	for _, node := range nodes {
		gomega.Eventually(func(g gomega.Gomega) int {
			return configupdate.ConfigSequenceOn(g, s.II, node)
		}, 60*time.Second, time.Second).Should(gomega.Equal(expected),
			"node [%s] never reported channel configuration sequence [%d]", node, expected)
	}
}

// TestSucceeded drives the IOU flow across three channel configuration changes, one of
// which lands while a node is down.
//
// Every IOU call goes through approver1: the fabricx namespace endorsement policy is
// Unanimity("Org1") and only approver1 is in Org1. approver2 is here to be asserted on,
// not to endorse.
func (s *TestSuite) TestSucceeded() {
	n := configupdate.NetworkOf(s.II)

	InitApprover(s.II, "approver1")
	InitApprover(s.II, "approver2")

	By("transacting on the channel's initial configuration")
	gomega.Expect(fxnetwork.GetConfig(n).GetSequence()).To(gomega.BeNumerically("==", 0))
	iouState := CreateIOU(s.II, 10, "approver1")
	CheckState(s.II, "borrower", iouState, 10)
	CheckState(s.II, "lender", iouState, 10)

	By("checking every node starts from the genesis configuration")
	expectConfigSequence(s, 0, configupdate.AllNodes...)

	By("changing the channel configuration while the FSC nodes are running")
	configupdate.SetBatchTimeout(n, 2*time.Second)
	gomega.Expect(configupdate.BatchTimeout(n)).To(gomega.Equal(2 * time.Second))

	By("checking every node applied the first configuration update")
	expectConfigSequence(s, 1, configupdate.AllNodes...)

	By("transacting again, which requires the nodes to have followed the change")
	second := CreateIOU(s.II, 20, "approver1")
	CheckState(s.II, "borrower", second, 20)
	CheckState(s.II, "lender", second, 20)

	By("changing the configuration a second time, so more than one update is on the ledger")
	configupdate.SetBatchTimeout(n, 500*time.Millisecond)
	gomega.Expect(configupdate.BatchTimeout(n)).To(gomega.Equal(500 * time.Millisecond))

	By("checking every node applied the second configuration update")
	expectConfigSequence(s, 2, configupdate.AllNodes...)

	third := CreateIOU(s.II, 30, "approver1")
	CheckState(s.II, "borrower", third, 30)
	CheckState(s.II, "lender", third, 30)

	By("changing the configuration a third time, while one node is down")
	// The fabric platform replays stored configuration transactions from the node's vault
	// on start-up. Fabricx has no such replay: the config monitor's first poll fetches
	// only the latest configuration. What is worth testing here is therefore not that a
	// restarted node re-applies what it already had, but that a node absent for an update
	// converges on the configuration it missed.
	s.II.StopFSCNode("borrower")
	configupdate.SetBatchTimeout(n, 3*time.Second)
	gomega.Expect(configupdate.BatchTimeout(n)).To(gomega.Equal(3 * time.Second))
	s.II.StartFSCNode("borrower")

	By("checking the node that was down caught up on the configuration it missed")
	expectConfigSequence(s, 3, configupdate.AllNodes...)

	By("transacting once more through the node that was down")
	fourth := CreateIOU(s.II, 40, "approver1")
	CheckState(s.II, "borrower", fourth, 40)
	CheckState(s.II, "lender", fourth, 40)
}

// TestRefused submits a configuration the FSC nodes cannot support and checks that they go
// on serving the one they already hold.
//
// It first applies an ordinary configuration update and waits for every node to adopt it.
// That step is not redundant with Spec 1: it is what makes the later "nodes stay put"
// assertion mean "the nodes refused the configuration" rather than "the nodes never saw
// it". Without it, a stalled or dead config monitor would leave every node at sequence 0
// regardless of the poisoned update, and the spec would pass for the wrong reason. Proving
// the monitor live on this same network, moments before the poisoned update, closes that
// gap -- do not remove it as apparent duplication of Spec 1.
//
// This spec runs on its own network because the refused configuration cannot be recovered
// from in place: every update is computed against whatever the committer currently serves,
// so a later update would inherit the unsupported capability and be refused too. The good
// update above must come first for the same reason: computed after the poisoned one, it
// would inherit V99_0 and be refused too.
//
// It is also expected to produce a noisy log. On a failed apply the monitor returns before
// updating lastVersion and lastSequence (monitor.go:236), so it re-fetches and re-refuses
// the same configuration for as long as the network runs. checkAndUpdate wraps the fetch
// and apply in retryWithBackoff, so one round refuses the configuration six times, sleeping
// 1+2+4+8+16 seconds between attempts -- about half a minute -- and because the poll loop
// calls it synchronously on a one-second ticker, the next round starts as soon as the
// previous one gives up. That is current behaviour, not a fault in this test.
func (s *TestSuite) TestRefused() {
	n := configupdate.NetworkOf(s.II)

	InitApprover(s.II, "approver1")

	By("transacting on the channel's initial configuration")
	iouState := CreateIOU(s.II, 10, "approver1")
	CheckState(s.II, "borrower", iouState, 10)
	expectConfigSequence(s, 0, configupdate.AllNodes...)

	By("applying a configuration the nodes accept, so a stalled monitor fails here")
	configupdate.SetBatchTimeout(n, 2*time.Second)
	expectConfigSequence(s, 1, configupdate.AllNodes...)

	By("submitting a configuration requiring a capability no node supports")
	configupdate.RequireUnsupportedCapability(n)

	// Asserting that the committer accepted it is what stops this spec from passing for
	// the wrong reason: were the committer to start refusing the capability itself, every
	// node would stay at sequence 1 and the spec would still be green.
	By("checking the committer accepted the configuration")
	gomega.Eventually(func() uint64 {
		return fxnetwork.GetConfig(n).GetSequence()
	}, 60*time.Second, time.Second).Should(gomega.BeNumerically("==", 2))

	By("checking no node adopts it")
	for _, node := range configupdate.AllNodes {
		gomega.Consistently(func(g gomega.Gomega) int {
			return configupdate.ConfigSequenceOn(g, s.II, node)
		}, 10*time.Second, time.Second).Should(gomega.Equal(1),
			"node [%s] adopted a configuration it cannot support", node)
	}

	By("checking the nodes still transact on the configuration they kept")
	second := CreateIOU(s.II, 20, "approver1")
	CheckState(s.II, "borrower", second, 20)
	CheckState(s.II, "lender", second, 20)
}
