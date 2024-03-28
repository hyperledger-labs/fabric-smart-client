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
		s := NewTestSuite(fsc.LibP2P, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("Echo FPC With Websockets", func() {
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
		return integration.Generate(StartPort(), true, echo.Topology(&fabric.SDK{}, commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	provisionedEnclavesBoxed, err := s.II.Client("alice").CallView(
		"ListProvisionedEnclaves",
		common.JSONMarshall(&views.ListProvisionedEnclaves{
			CID: "echo",
		}),
	)
	Expect(err).ToNot(HaveOccurred())
	var provisionedEnclaves []string
	common.JSONUnmarshal(provisionedEnclavesBoxed.([]byte), &provisionedEnclaves)
	Expect(len(provisionedEnclaves)).To(BeEquivalentTo(1))

	resBoxed, err := s.II.Client("alice").CallView(
		"Echo",
		common.JSONMarshall(&views.Echo{
			Function: "myFunction",
			Args:     []string{"arg1", "arg2", "arg3"},
		}),
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(resBoxed).To(BeEquivalentTo("myFunction"))
}
