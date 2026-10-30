/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/stoprestart"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("EndToEnd", func() {
	// Both transports: session and stream recovery after a peer restart is
	// transport-sensitive, and this suite needs no Fabric network to check it.
	for _, c := range []fsc.P2PCommunicationType{fsc.WebSocket, fsc.LibP2P} {
		Describe("Stop and Restart", Label(c), func() {
			s := NewTestSuite(c)
			BeforeEach(s.Setup)
			AfterEach(s.TearDown)
			It("answers a view call after the responder restarts", s.TestSucceeded)
			It("finishes a running view when its responder stops gracefully", s.TestDrains)
		})
	}
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), integration.WithRaceDetection, stoprestart.Topology(commType)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	res, err := s.II.Client("alice").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	s.II.StopFSCNode("bob")
	s.II.StartFSCNode("bob")

	res, err = s.II.Client("alice").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	m, err := s.II.WebClient("alice").Metrics()
	Expect(err).NotTo(HaveOccurred())
	Expect(m).NotTo(BeNil())
}

// TestDrains stops bob while its responder runs: the stop drains, so the view completes.
func (s *TestSuite) TestDrains() {
	res := make(chan any, 1)
	go func() {
		defer GinkgoRecover()
		r, err := s.II.Client("alice").CallView("slow", nil)
		Expect(err).NotTo(HaveOccurred())
		res <- r
	}()
	time.Sleep(stoprestart.SlowResponderDelay / 3) // the responder runs

	s.II.StopFSCNode("bob")
	Eventually(res, 2*stoprestart.SlowResponderDelay).Should(Receive(WithTransform(common.JSONUnmarshalString, BeEquivalentTo("OK"))))
	s.II.StartFSCNode("bob")
}
