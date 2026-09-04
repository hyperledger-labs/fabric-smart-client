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
	// Both transports: a signed round-trip is cheap to run twice -- this suite
	// starts no Fabric network.
	for _, c := range []fsc.P2PCommunicationType{fsc.WebSocket, fsc.LibP2P} {
		Describe("Network-based Signed Ping Pong", Label(c), func() {
			s := NewTestSuite(c, integration.NoReplication)
			BeforeEach(s.Setup)
			AfterEach(s.TearDown)
			It("signed exchange succeeds", func() { s.TestSignedExchange("alice") })
		})
	}
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
