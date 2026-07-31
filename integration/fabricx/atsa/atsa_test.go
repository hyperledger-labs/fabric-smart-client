/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package atsa_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/states"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/atsa"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("EndToEnd", func() {
	for _, c := range []nwofsc.P2PCommunicationType{nwofsc.WebSocket} {
		Describe("Asset Transfer Secured Agreement", Label(c), func() {
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
			integration.FabricXAssetTransferPort.StartPortForNode(),
			"",
			atsa.Topology(&atsa.SDK{}, commType, nodeOpts)...,
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
	approver := s.II.Identity("approver")

	issuer := client.New(s.II.Client("issuer"), s.II.Identity("issuer"), approver)
	_, err := issuer.Issue(&states.Asset{
		ObjectType:        "coin",
		ID:                "1234",
		Owner:             s.II.Identity("alice"),
		PublicDescription: "Coin",
		PrivateProperties: []byte("Hello World!!!"),
	})
	Expect(err).ToNot(HaveOccurred())

	// Note: unlike the Fabric variant, we do not issue a standalone client-side
	// finality check here. Fabric-x finality is delivered once over the committer's
	// ephemeral notification stream and is already awaited in-view by both the
	// issuer (NewOrderingAndFinalityView) and alice (AcceptAssetView), so a fresh
	// post-commit IsFinal for the same txID would time out as "unknown".
	seller := client.New(s.II.Client("alice"), s.II.Identity("alice"), approver)
	agreementID, err := seller.AgreeToSell(&states.AgreementToSell{
		TradeID: "1234",
		ID:      "1234",
		Price:   100,
	})
	Expect(err).ToNot(HaveOccurred())

	buyer := client.New(s.II.Client("bob"), s.II.Identity("bob"), approver)
	_, err = buyer.AgreeToBuy(&states.AgreementToBuy{
		TradeID: "1234",
		ID:      "1234",
		Price:   100,
	})
	Expect(err).ToNot(HaveOccurred())

	err = seller.Transfer("1234", agreementID, s.II.Identity("bob"))
	Expect(err).ToNot(HaveOccurred())
}
