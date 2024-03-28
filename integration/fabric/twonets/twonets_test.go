/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package twonets_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/twonets"
)

var _ = Describe("EndToEnd", func() {
	Describe("Two Fabric Networks Life Cycle With LibP2P", func() {
		s := NewTestSuite(fsc.LibP2P, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("Two Fabric Networks Life Cycle With Websockets", func() {
		s := NewTestSuite(fsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), true, twonets.Topology(&fabric.SDK{}, commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	res, err := s.II.Client("alice").CallView("ping", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}
