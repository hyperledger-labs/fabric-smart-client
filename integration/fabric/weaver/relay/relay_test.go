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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	Describe("Two Fabric Networks with Weaver Relay Life Cycle With Websockets", func() {
		s := NewTestSuite(fsc.WebSocket, integration.NoReplication)
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

const testdataPath = "./testdata"

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.GenerateAt(StartPort(), testdataPath, true, relay.Topology(&relay.SDK{}, commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	res, err := s.II.Client("alice").CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

	// cleanup testdata when test succeeds.
	// if the test fails, we keep the test data for reviewing what went wrong
	err = os.RemoveAll(testdataPath)
	Expect(err).NotTo(HaveOccurred())
}
