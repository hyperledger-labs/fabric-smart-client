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
		s := TestSuite{commType: fsc.LibP2P}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("stop and restart successfully", s.TestSucceeded)
	})

	Describe("Stop and Restart With Websockets", func() {
		s := TestSuite{commType: fsc.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("stop and restart successfully", s.TestSucceeded)
	})
})

type TestSuite struct {
	commType fsc.P2PCommunicationType

	ii *integration.Infrastructure
}

func (s *TestSuite) TearDown() {
	s.ii.Stop()
}

func (s *TestSuite) Setup() {
	// Create the integration ii
	ii, err := integration.Generate(StartPort(), true, stoprestart.Topology(s.commType)...)
	Expect(err).NotTo(HaveOccurred())
	s.ii = ii
	// Start the integration ii
	ii.Start()
	time.Sleep(3 * time.Second)
}

func (s *TestSuite) TestSucceeded() {
	res, err := s.ii.Client("alice").CallView("init", nil)
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
