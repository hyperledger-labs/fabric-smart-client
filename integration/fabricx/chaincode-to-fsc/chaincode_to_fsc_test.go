/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincodetofsc_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	chaincodetofsc "github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc/states"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc/views"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("EndToEnd", func() {
	for _, c := range []nwofsc.P2PCommunicationType{nwofsc.WebSocket} {
		Describe("Chaincode-to-FSC Asset Transfer", Label(c), func() {
			s := NewTestSuite(c)
			BeforeEach(s.Setup)
			AfterEach(s.TearDown)

			It("ports the eight asset-transfer-basic chaincode methods to FSC views",
				s.TestEndToEnd)
		})
	}
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType nwofsc.P2PCommunicationType) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(
			integration.IOUPort.StartPortForNode(),
			"",
			chaincodetofsc.Topology(&chaincodetofsc.SDK{}, commType)...,
		)
		if err != nil {
			return nil, err
		}
		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
		ii.Generate()
		return ii, nil
	})}
}

// TestEndToEnd is the single integration test that exercises every ported
// chaincode method against a real Fabric-X network started by NWO.
//
// The test reads as a tutorial: each By(...) block names the chaincode
// method being demonstrated, the equivalent FSC view, and the expected
// outcome. Reviewers (and a future mentee inheriting this code) can read
// the test top-to-bottom to learn the migration story.
func (s *TestSuite) TestEndToEnd() {
	By("Bootstrapping the endorser to process the asset-transfer namespace")
	initEndorser(s.II)

	By("InitLedger — chaincode wrote 6 assets via PutState; FSC writes 6 outputs in one tx")
	initLedger(s.II)

	By("ReadAsset — chaincode used GetState; FSC uses Query Service")
	a1 := readAsset(s.II, chaincodetofsc.IssuerNode, "asset1")
	Expect(a1).To(Equal(&states.Asset{
		ID: "asset1", Color: "blue", Size: 5, Owner: "Tomoko", AppraisedValue: 300,
	}))

	By("AssetExists — chaincode read+nil-check; FSC same shape, no unmarshal")
	Expect(assetExists(s.II, chaincodetofsc.IssuerNode, "asset1")).To(BeTrue())
	Expect(assetExists(s.II, chaincodetofsc.IssuerNode, "doesNotExist")).To(BeFalse())

	By("CreateAsset — happy path: brand-new ID")
	newAsset := &states.Asset{
		ID: "asset7", Color: "purple", Size: 7, Owner: "alice", AppraisedValue: 777,
	}
	createAsset(s.II, chaincodetofsc.IssuerNode, newAsset)
	Expect(readAsset(s.II, chaincodetofsc.IssuerNode, "asset7")).To(Equal(newAsset))

	By("CreateAsset — negative path: duplicate ID is rejected by the endorser")
	createAssetExpectFail(s.II, chaincodetofsc.IssuerNode, newAsset)

	By("UpdateAsset — chaincode existence-check + PutState; FSC uses AddInputByLinearID")
	updated := &states.Asset{
		ID: "asset7", Color: "indigo", Size: 9, Owner: "alice", AppraisedValue: 999,
	}
	updateAsset(s.II, chaincodetofsc.AliceNode, updated)
	Expect(readAsset(s.II, chaincodetofsc.AliceNode, "asset7")).To(Equal(updated))

	By("TransferAsset — chaincode mutated Owner unilaterally; FSC requires receiver acceptance")
	old := transferAsset(s.II, chaincodetofsc.AliceNode, "asset7", "bob", chaincodetofsc.BobNode)
	Expect(old).To(Equal("alice"), "TransferAsset must return the previous owner string")
	after := readAsset(s.II, chaincodetofsc.BobNode, "asset7")
	Expect(after.Owner).To(Equal("bob"))
	Expect(after.Color).To(Equal("indigo"), "transfer must not change Color")
	Expect(after.Size).To(Equal(9), "transfer must not change Size")
	Expect(after.AppraisedValue).To(Equal(999), "transfer must not change AppraisedValue")

	By("TransferAsset — negative path: no-op transfer rejected at the initiator")
	transferAssetExpectFail(s.II, chaincodetofsc.BobNode, "asset7", "bob", chaincodetofsc.BobNode)

	By("DeleteAsset — chaincode existence-check + DelState; FSC: AddInputByLinearID, no output")
	deleteAsset(s.II, chaincodetofsc.BobNode, "asset7")
	Expect(assetExists(s.II, chaincodetofsc.IssuerNode, "asset7")).To(BeFalse())
	readAssetExpectFail(s.II, chaincodetofsc.IssuerNode, "asset7")

	By("GetAllAssets — chaincode used GetStateByRange; Fabric-X has no range query, FSC uses explicit-ID-list")
	allIDs := []string{"asset1", "asset2", "asset3", "asset4", "asset5", "asset6", "asset7"}
	all := getAllAssets(s.II, chaincodetofsc.IssuerNode, allIDs)
	// asset7 was deleted, the seed six remain
	Expect(all).To(HaveLen(6))
	seedIDs := make([]string, len(views.SeedAssets()))
	for i, a := range views.SeedAssets() {
		seedIDs[i] = a.ID
	}
	for _, a := range all {
		Expect(seedIDs).To(ContainElement(a.ID))
	}
}
