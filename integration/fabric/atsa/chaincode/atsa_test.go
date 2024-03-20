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
		s := TestSuite{commType: fsc.LibP2P}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("Asset Transfer Secured Agreement (With Chaincode) with WebSockets", func() {
		s := TestSuite{commType: fsc.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})
})

type TestSuite struct {
	commType fsc.P2PCommunicationType

	ii *integration.Infrastructure

	alice *chaincode.Client
	bob   *chaincode.Client
}

func (s *TestSuite) TearDown() {
	s.ii.Stop()
}

func (s *TestSuite) Setup() {
	// Create the integration ii
	ii, err := integration.Generate(StartPort(), true, chaincode.Topology(&fabric.SDK{}, s.commType)...)
	Expect(err).NotTo(HaveOccurred())
	s.ii = ii
	// Start the integration ii
	ii.Start()

	s.alice = chaincode.NewClient(ii.Client("alice"), ii.Identity("alice"))
	s.bob = chaincode.NewClient(ii.Client("bob"), ii.Identity("bob"))
}

func (s *TestSuite) TestSucceeded() {
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
	Expect(s.alice.CreateAsset(ap, "A new asset for Org1MSP")).ToNot(HaveOccurred())

	ap2, err := s.alice.ReadAssetPrivateProperties(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(ap2).To(BeEquivalentTo(ap))

	asset, err := s.alice.ReadAsset(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(asset.ID).To(BeEquivalentTo(ap.ID))
	Expect(asset.ObjectType).To(BeEquivalentTo("asset"))
	Expect(asset.PublicDescription).To(BeEquivalentTo("A new asset for Org1MSP"))
	Expect(asset.OwnerOrg).To(BeEquivalentTo("Org1MSP"))

	Expect(s.alice.ChangePublicDescription(ap.ID, "This asset is for sale")).ToNot(HaveOccurred())
	asset, err = s.alice.ReadAsset(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(asset.ID).To(BeEquivalentTo(ap.ID))
	Expect(asset.ObjectType).To(BeEquivalentTo("asset"))
	Expect(asset.PublicDescription).To(BeEquivalentTo("This asset is for sale"))

	// - Operate from Bob (Org2)
	asset, err = s.bob.ReadAsset(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(asset.ID).To(BeEquivalentTo(ap.ID))
	Expect(asset.ObjectType).To(BeEquivalentTo("asset"))
	Expect(asset.PublicDescription).To(BeEquivalentTo("This asset is for sale"))
	Expect(asset.OwnerOrg).To(BeEquivalentTo("Org1MSP"))

	Expect(s.bob.ChangePublicDescription(ap.ID, "This asset is NOT for sale")).To(HaveOccurred())

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
	err = s.alice.AgreeToSell(assetPriceSell)
	Expect(err).ToNot(HaveOccurred())

	// Bob (Org2) agree to buy
	assetPriceBuy := &views.AssetPrice{
		AssetID: ap.ID,
		TradeID: tradeID,
		Price:   100,
	}
	err = s.bob.AgreeToBuy(assetPriceBuy)
	Expect(err).ToNot(HaveOccurred())

	// Transfer the asset from Alice (Org1) to Bob (Org2)
	err = s.alice.Transfer(ap, assetPriceSell, s.bob.Identity())
	Expect(err).To(HaveOccurred())

	// Alice (Org1) agree to sell
	assetPriceSell.Price = 100
	err = s.alice.AgreeToSell(assetPriceSell)
	Expect(err).ToNot(HaveOccurred())

	// Transfer the asset from Alice (Org1) to Bob (Org2)
	err = s.alice.Transfer(ap, assetPriceSell, s.bob.Identity())
	Expect(err).ToNot(HaveOccurred())

	// Update the asset description as Bob (Org2)
	asset, err = s.bob.ReadAsset(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(asset.ID).To(BeEquivalentTo(ap.ID))
	Expect(asset.ObjectType).To(BeEquivalentTo("asset"))
	Expect(asset.PublicDescription).To(BeEquivalentTo("This asset is for sale"))
	Expect(asset.OwnerOrg).To(BeEquivalentTo("Org2MSP"))

	ap2, err = s.bob.ReadAssetPrivateProperties(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(ap2).To(BeEquivalentTo(ap))

	Expect(s.bob.ChangePublicDescription(ap.ID, "This asset is not for sale")).ToNot(HaveOccurred())
	asset, err = s.bob.ReadAsset(ap.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(asset.ID).To(BeEquivalentTo(ap.ID))
	Expect(asset.ObjectType).To(BeEquivalentTo("asset"))
	Expect(asset.PublicDescription).To(BeEquivalentTo("This asset is not for sale"))
}
