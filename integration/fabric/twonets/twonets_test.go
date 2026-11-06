/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package twonets_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/twonets"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

var _ = Describe("EndToEnd", func() {
	// websocket only: the p2p exchange here is a single ping/pong that
	// fsc/pingpong covers without a Fabric network. This suite's subject is two
	// Fabric networks, and it pays two bootstraps per spec.
	Describe("Two Fabric Networks Life Cycle", Label(fsc.WebSocket), func() {
		s := NewTestSuite(integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("pings a node on two Fabric networks, queries both and commits on each", s.TestSucceeded)
		It("endorses a transaction received for the responder's non-default network", s.TestEndorseOnNonDefaultNetwork)
		It("keeps committing on alpha while beta's peers are down", s.TestOutageOnOneNetwork)
	})
})

// betaPeers are the peers of beta, one per organization.
var betaPeers = []string{"Org3_peer_0", "Org4_peer_0"}

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), true, twonets.Topology(nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	res, err := s.II.Client("alice").CallView("ping", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	// Both broadcasters count ordered transactions alike, so the orderer connections tell
	// them apart. The BFT broadcaster sends the envelope to all four orderers of beta, so
	// the commit leaves a series for each. It returns once three of them accepted it, so
	// the last series may appear after the commit. The CFT broadcaster sends it to one
	// orderer per attempt, at most 3 with nwo's numRetries.
	betaOrderers := fsc.Connection{Network: "beta", Component: connstate.ComponentOrderer}
	Eventually(func(g Gomega) {
		families, err := s.II.WebClient("alice").Metrics()
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(fsc.ParseConnectionState(families, betaOrderers)).To(HaveLen(4), "alice must have broadcast to every orderer of beta")
	}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Succeed())
}

// TestEndorseOnNonDefaultNetwork has each node build a transaction on its default
// network and collect the other node's endorsement, for which that network is not
// the default. The transaction must commit on the network it was built on, as seen
// from the vault of that network on both nodes.
func (s *TestSuite) TestEndorseOnNonDefaultNetwork() {
	twonets.EndorseAcross(s.II, []twonets.Endorsement{
		{Initiator: "alice", Endorser: "bob", Network: "alpha", Namespace: "ns1"},
		{Initiator: "bob", Endorser: "alice", Network: "beta", Namespace: "ns2"},
	})
}

// TestOutageOnOneNetwork stops every peer of beta. Alice's beta delivery must report
// Reconnecting while her alpha delivery stays Connected and a transaction commits on alpha.
// Once the peers are back, beta's delivery reconnects and a transaction commits on beta.
func (s *TestSuite) TestOutageOnOneNetwork() {
	alphaDelivery := fsc.Connection{Network: "alpha", Component: connstate.ComponentDelivery}
	betaDelivery := fsc.Connection{Network: "beta", Component: connstate.ComponentDelivery}
	// Alice's delivery has one peer per network, that of her organization (Org1_peer_0,
	// Org3_peer_0), so each selects a single series.
	s.II.EventuallyConnectionState("alice", alphaDelivery, connstate.Connected, time.Minute)
	s.II.EventuallyConnectionState("alice", betaDelivery, connstate.Connected, time.Minute)

	for _, p := range betaPeers {
		s.II.StopPeer(p)
	}
	s.II.EventuallyConnectionState("alice", betaDelivery, connstate.Reconnecting, time.Minute)

	twonets.EndorseAcross(s.II, []twonets.Endorsement{
		{Initiator: "alice", Endorser: "bob", Network: "alpha", Namespace: "ns1"},
	})
	s.II.EventuallyConnectionState("alice", alphaDelivery, connstate.Connected, time.Minute)

	for _, p := range betaPeers {
		s.II.StartPeer(p)
	}
	s.II.EventuallyConnectionState("alice", betaDelivery, connstate.Connected, faultrecovery.RecoveryTimeout)
	// A peer that is up may not serve its channel yet, so the first endorsement on beta retries.
	twonets.EndorseAcrossEventually(s.II, []twonets.Endorsement{
		{Initiator: "bob", Endorser: "alice", Network: "beta", Namespace: "ns2"},
	}, faultrecovery.RecoveryTimeout)
}
