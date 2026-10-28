/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signedpingpong_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/signedpingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("EndToEnd", func() {
	// websocket only: the views sign and verify at the application layer. The
	// transport-specific part, resolving a PKID to an identity through the
	// endpoint service, runs for every inbound libp2p session in fsc/pingpong,
	// and comm/host/libp2p/sig_exchange_test.go covers the signed exchange,
	// including LocalPKID, on libp2p hosts.
	Describe("Network-based Signed Ping Pong", Label(fsc.WebSocket), func() {
		s := NewTestSuite(fsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("signs and verifies messages with the identities of the session's PKIDs", func() { s.TestSignedExchange("alice") })
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), integration.WithRaceDetection, signedpingpong.Topology(commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSignedExchange(clients ...string) {
	for _, clientName := range clients {
		res, err := s.II.Client(clientName).CallView("signedInit", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
	}
}
