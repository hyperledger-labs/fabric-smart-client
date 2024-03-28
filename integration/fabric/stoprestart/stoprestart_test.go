/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart_test

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/stoprestart"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

var _ = Describe("EndToEnd", func() {
	Describe("Stop and Restart with Fabric With LibP2P", func() {
		s := TestSuite{commType: fsc.LibP2P}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("stop and restart successfully", s.TestSucceeded)
	})

	Describe("Stop and Restart with Fabric With Websockets", func() {
		s := TestSuite{commType: fsc.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("stop and restart successfully", s.TestSucceeded)
	})

	Describe("Stop and Restart with Fabric With Replicas many to one", func() {
		s := TestSuite{commType: fsc.WebSocket, replicas: map[string]int{"alice": 4, "bob": 1}}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("stop and restart successfully", s.TestSucceededWithReplicas)
	})

	Describe("Stop and Restart with Fabric With Replicas many to many", func() {
		s := TestSuite{commType: fsc.WebSocket, replicas: map[string]int{"alice": 4, "bob": 4}}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("stop and restart successfully", s.TestSucceededWithReplicas)
	})
})

type TestSuite struct {
	commType fsc.P2PCommunicationType
	replicas map[string]int

	ii *integration.Infrastructure
}

func (s *TestSuite) TearDown() {
	s.ii.Stop()
}

func (s *TestSuite) Setup() {
	// Create the integration ii
	ii, err := integration.Generate(StartPort(), true, stoprestart.Topology(&fabric.SDK{}, s.commType, s.replicas)...)
	Expect(err).NotTo(HaveOccurred())
	s.ii = ii
	// Start the integration ii
	ii.Start()
	time.Sleep(3 * time.Second)
}

func (s *TestSuite) TestSucceeded() {
	res, err := s.ii.CLI("alice").CallView("init", []byte("foo"))
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	s.ii.StopFSCNode("bob")
	time.Sleep(3 * time.Second)
	s.ii.StartFSCNode("bob")
	time.Sleep(3 * time.Second)

	res, err = s.ii.Client("alice").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}

func (s *TestSuite) TestSucceededWithReplicas() {
	res, err := s.ii.Client("fsc.alice.0").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	res, err = s.ii.Client("fsc.alice.1").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	res, err = s.ii.Client("fsc.alice.2").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	s.ii.StopFSCNode("bob")
	time.Sleep(3 * time.Second)
	s.ii.StartFSCNode("bob")
	time.Sleep(3 * time.Second)

	res, err = s.ii.Client("fsc.alice.0").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	res, err = s.ii.Client("fsc.alice.1").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	res, err = s.ii.Client("fsc.alice.2").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}
