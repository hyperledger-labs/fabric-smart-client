/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tokenx_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/states"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	for _, c := range []nwofsc.P2PCommunicationType{nwofsc.WebSocket} {
		Describe("Token Life Cycle", Label(c), func() {
			s := NewTestSuite(c, integration.NoReplication)
			BeforeEach(s.Setup)
			AfterEach(s.TearDown)

			It("succeeded", s.TestSucceeded)
		})
	}
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType nwofsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(
			integration.IOUPort.StartPortForNode(),
			"",
			tokenx.Topology(&tokenx.SDK{}, commType, nodeOpts, "alice", "bob", "charlie")...,
		)
		if err != nil {
			return nil, err
		}

		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
		ii.Generate()

		return ii, nil
	})}
}

func (s *TestSuite) TestSucceeded() {
	// Issue USD tokens to Alice (following simple pattern)
	By("issuing USD tokens to alice")
	usdTokenID := IssueTokens(s.II, "USD", states.TokenFromFloat(1000), "alice")
	Expect(usdTokenID).NotTo(BeEmpty())

	// Issue EUR tokens to Bob
	By("issuing EUR tokens to bob")
	eurTokenID := IssueTokens(s.II, "EUR", states.TokenFromFloat(500), "bob")
	Expect(eurTokenID).NotTo(BeEmpty())

	// Issue GOLD tokens to Charlie
	By("issuing GOLD tokens to charlie")
	goldTokenID := IssueTokens(s.II, "GOLD", states.TokenFromFloat(10), "charlie")
	Expect(goldTokenID).NotTo(BeEmpty())
}

// Helper functions

func IssueTokens(ii *integration.Infrastructure, tokenType string, amount uint64, recipient string) string {
	res, err := ii.Client(tokenx.IssuerNode).CallView("issue", common.JSONMarshall(&views.Issue{
		TokenType: tokenType,
		Amount:    amount,
		Recipient: ii.Identity(recipient),
		Approvers: []view.Identity{ii.Identity(tokenx.ApproverNode)},
	}))
	Expect(err).NotTo(HaveOccurred())
	return common.JSONUnmarshalString(res)
}
