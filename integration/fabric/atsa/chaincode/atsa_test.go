/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"encoding/base64"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/chaincode/views"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
)

var _ = Describe("EndToEnd", func() {
	Describe("Asset Transfer Secured Agreement (With Chaincode) with LibP2P", func() {
		s := NewTestSuite(fsc.LibP2P, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("Asset Transfer Secured Agreement (With Chaincode) with WebSockets", func() {
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
		return integration.Generate(StartPort(), true, chaincode.Topology(&fabric.SDK{}, commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	alice := chaincode.NewClient(s.II.Client("alice"), s.II.Identity("alice"))
	bob := chaincode.NewClient(s.II.Client("bob"), s.II.Identity("bob"))
	// Create an asset

	// - Operate from Alice (Org1)
	nonce, err := state.CreateNonce()
	Expect(err).ToNot(HaveOccurred())
	ap := &views.AssetProperties{
		ObjectType: "asset_properties",
		ID:         "asset1",
		Color:      "blue",
		Size:       35,
		Salt:       nonce,
	}
	Expect(alice.CreateAsset(ap, "A new asset for Org1MSP")).ToNot(HaveOccurred())

	ap2, err := alice.ReadAssetPrivateProperties(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(ap2).To(BeEquivalentTo(ap))

	asset, err := alice.ReadAsset(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(asset.ID).To(BeEquivalentTo(ap.ID))
	Expect(asset.ObjectType).To(BeEquivalentTo("asset"))
	Expect(asset.PublicDescription).To(BeEquivalentTo("A new asset for Org1MSP"))
	Expect(asset.OwnerOrg).To(BeEquivalentTo("Org1MSP"))

	Expect(alice.ChangePublicDescription(ap.ID, "This asset is for sale")).ToNot(HaveOccurred())
	asset, err = alice.ReadAsset(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(asset.ID).To(BeEquivalentTo(ap.ID))
	Expect(asset.ObjectType).To(BeEquivalentTo("asset"))
	Expect(asset.PublicDescription).To(BeEquivalentTo("This asset is for sale"))

	// - Operate from Bob (Org2)
	asset, err = bob.ReadAsset(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(asset.ID).To(BeEquivalentTo(ap.ID))
	Expect(asset.ObjectType).To(BeEquivalentTo("asset"))
	Expect(asset.PublicDescription).To(BeEquivalentTo("This asset is for sale"))
	Expect(asset.OwnerOrg).To(BeEquivalentTo("Org1MSP"))

	Expect(bob.ChangePublicDescription(ap.ID, "This asset is NOT for sale")).To(HaveOccurred())

	// Agree to sell the asset

	nonce, err = state.CreateNonce()
	Expect(err).ToNot(HaveOccurred())
	tradeID := base64.StdEncoding.EncodeToString(nonce)

	// Alice (Org1) agree to sell
	assetPriceSell := &views.AssetPrice{
		AssetID: ap.ID,
		TradeID: tradeID,
		Price:   110,
	}
	err = alice.AgreeToSell(assetPriceSell)
	Expect(err).ToNot(HaveOccurred())

	// Bob (Org2) agree to buy
	assetPriceBuy := &views.AssetPrice{
		AssetID: ap.ID,
		TradeID: tradeID,
		Price:   100,
	}
	err = bob.AgreeToBuy(assetPriceBuy)
	Expect(err).ToNot(HaveOccurred())

	// Transfer the asset from Alice (Org1) to Bob (Org2)
	err = alice.Transfer(ap, assetPriceSell, bob.Identity())
	Expect(err).To(HaveOccurred())

	// Alice (Org1) agree to sell
	assetPriceSell.Price = 100
	err = alice.AgreeToSell(assetPriceSell)
	Expect(err).ToNot(HaveOccurred())

	// Transfer the asset from Alice (Org1) to Bob (Org2)
	err = alice.Transfer(ap, assetPriceSell, bob.Identity())
	Expect(err).ToNot(HaveOccurred())

	// Update the asset description as Bob (Org2)
	asset, err = bob.ReadAsset(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(asset.ID).To(BeEquivalentTo(ap.ID))
	Expect(asset.ObjectType).To(BeEquivalentTo("asset"))
	Expect(asset.PublicDescription).To(BeEquivalentTo("This asset is for sale"))
	Expect(asset.OwnerOrg).To(BeEquivalentTo("Org2MSP"))

	ap2, err = bob.ReadAssetPrivateProperties(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(ap2).To(BeEquivalentTo(ap))

	Expect(bob.ChangePublicDescription(ap.ID, "This asset is not for sale")).ToNot(HaveOccurred())
	asset, err = bob.ReadAsset(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(asset.ID).To(BeEquivalentTo(ap.ID))
	Expect(asset.ObjectType).To(BeEquivalentTo("asset"))
	Expect(asset.PublicDescription).To(BeEquivalentTo("This asset is not for sale"))
}
