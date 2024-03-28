/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart_test

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/stoprestart"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

var _ = Describe("EndToEnd", func() {
	Describe("Stop and Restart With LibP2P", func() {
		s := NewTestSuite(fsc.LibP2P, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("stop and restart successfully", s.TestSucceeded)
	})

	Describe("Stop and Restart With Websockets", func() {
		s := NewTestSuite(fsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("stop and restart successfully", s.TestSucceeded)
	})

	Describe("Stop and Restart with Fabric With Replicas many to one", func() {
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

	Describe("Stop and Restart with Fabric With Replicas many to many", func() {
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
		return integration.Generate(StartPort(), true, stoprestart.Topology(commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	res, err := s.II.Client("alice").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	s.II.StopFSCNode("bob")
	time.Sleep(3 * time.Second)
	s.II.StartFSCNode("bob")
	time.Sleep(3 * time.Second)

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
	time.Sleep(3 * time.Second)
	s.II.StartFSCNode("bob")
	time.Sleep(3 * time.Second)

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
