/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package relay_test

import (
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/weaver/relay"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	Describe("Two Fabric Networks with Weaver Relay Life Cycle With Websockets", func() {
		s := TestSuite{commType: fsc.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	//Describe("Two Fabric Networks with Weaver Relay Life Cycle With LibP2P", func() {
	//	s := TestSuite{commType: fsc.LibP2P}
	//	BeforeEach(s.Setup)
	//	AfterEach(s.TearDown)
	//	It("succeeded", s.TestSucceeded)
	//})
})

type TestSuite struct {
	commType fsc.P2PCommunicationType
	replicas map[string]int

	ii *integration.Infrastructure
}

func (s *TestSuite) TearDown() {
	s.ii.Stop()
}

const testdataPath = "./testdata"

func (s *TestSuite) Setup() {
	// Create the integration ii
	ii, err := integration.GenerateAt(StartPort(), testdataPath, true, relay.Topology(&fabric.SDK{}, s.commType, s.replicas)...)
	Expect(err).NotTo(HaveOccurred())
	s.ii = ii
	// Start the integration ii
	ii.Start()
}

func (s *TestSuite) TestSucceeded() {
	res, err := s.ii.Client("alice").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	// cleanup testdata when test succeeds.
	// if the test fails, we keep the test data for reviewing what went wrong
	err = os.RemoveAll(testdataPath)
	Expect(err).NotTo(HaveOccurred())
}
