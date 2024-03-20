/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package echo_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/fpc/echo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/fpc/echo/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

var _ = Describe("EndToEnd", func() {
	Describe("Echo FPC With LibP2P", func() {
		s := TestSuite{commType: fsc.LibP2P}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("Echo FPC With Websockets", func() {
		s := TestSuite{commType: fsc.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
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
	ii, err := integration.Generate(StartPort(), true, echo.Topology(&fabric.SDK{}, s.commType)...)
	Expect(err).NotTo(HaveOccurred())
	s.ii = ii
	// Start the integration ii
	ii.Start()
}

func (s *TestSuite) TestSucceeded() {
	provisionedEnclavesBoxed, err := s.ii.Client("alice").CallView(
		"ListProvisionedEnclaves",
		common.JSONMarshall(&views.ListProvisionedEnclaves{
			CID: "echo",
		}),
	)
	Expect(err).ToNot(HaveOccurred())
	var provisionedEnclaves []string
	common.JSONUnmarshal(provisionedEnclavesBoxed.([]byte), &provisionedEnclaves)
	Expect(len(provisionedEnclaves)).To(BeEquivalentTo(1))

	resBoxed, err := s.ii.Client("alice").CallView(
		"Echo",
		common.JSONMarshall(&views.Echo{
			Function: "myFunction",
			Args:     []string{"arg1", "arg2", "arg3"},
		}),
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(resBoxed).To(BeEquivalentTo("myFunction"))
}
