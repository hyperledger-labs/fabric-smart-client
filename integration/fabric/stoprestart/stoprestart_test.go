/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/stoprestart"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("EndToEnd", func() {
	// websocket only: fsc/stoprestart runs the same initiator/responder restart
	// scenario under libp2p without paying for a Fabric network.
	for _, c := range []fsc.P2PCommunicationType{fsc.WebSocket} {
		Describe("Stop and Restart with Fabric", Label(c), func() {
			s := NewTestSuite(c, integration.NoReplication)
			BeforeEach(s.Setup)
			AfterEach(s.TearDown)
			It("stop and restart successfully", s.TestSucceeded)
		})
	}

	Describe("Stop and Restart with Fabric With Replicas many to one", Label(fsc.WebSocket), func() {
		s := NewTestSuite(fsc.WebSocket, &integration.ReplicationOptions{
			ReplicationFactors: map[string]int{
				"alice": 4,
				"bob":   1,
			},
		})
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("stop and restart successfully", s.TestSucceededWithReplicas)
	})

	Describe("Stop and Restart with Fabric With Replicas many to many", Label(fsc.WebSocket), func() {
		s := NewTestSuite(fsc.WebSocket, &integration.ReplicationOptions{
			ReplicationFactors: map[string]int{
				"alice": 4,
				"bob":   4,
			},
		})
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("stop and restart successfully", s.TestSucceededWithReplicas)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), integration.WithRaceDetection, stoprestart.Topology(commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	res, err := s.II.CLI("alice").CallView("init", []byte("foo"))
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	s.II.StopFSCNode("bob")
	s.II.StartFSCNode("bob")

	res, err = s.II.Client("alice").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}

func (s *TestSuite) TestSucceededWithReplicas() {
	res, err := s.II.Client("fsc.alice.0").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	res, err = s.II.Client("fsc.alice.1").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	res, err = s.II.Client("fsc.alice.2").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	s.II.StopFSCNode("bob")
	s.II.StartFSCNode("bob")

	res, err = s.II.Client("fsc.alice.0").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	res, err = s.II.Client("fsc.alice.1").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	res, err = s.II.Client("fsc.alice.2").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}
