/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package twonets_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
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
		s := NewTestSuite(fsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("pings a node on two Fabric networks, queries both and commits on each", s.TestSucceeded)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), true, twonets.Topology(commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	res, err := s.II.Client("alice").CallView("ping", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	// Both broadcasters count ordered transactions alike, so the orderer connections tell
	// them apart. The BFT broadcaster sends the envelope to all four orderers of beta and
	// returns only after each send ends, so the commit leaves a series for each. The CFT
	// broadcaster sends it to one orderer per attempt, at most 3 with nwo's numRetries.
	families, err := s.II.WebClient("alice").Metrics()
	Expect(err).NotTo(HaveOccurred())
	betaOrderers := fsc.Connection{Network: "beta", Component: connstate.ComponentOrderer}
	Expect(fsc.ParseConnectionState(families, betaOrderers)).To(HaveLen(4), "alice must have broadcast to every orderer of beta")
}
