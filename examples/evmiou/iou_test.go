/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package evmiou_test

import (
	"context"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/eip712"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/chain"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/contract"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/nwo/evm"
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

// The specs share one network and run in order: each starts from the state the previous one left.
var _ = Describe("EVM IOU", Ordered, func() {
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(integration.EVMIOUPort.StartPortForNode(), "", evmiou.Topology()...)
		if err != nil {
			return nil, err
		}
		ii.RegisterPlatformFactory(evm.NewPlatformFactory())
		ii.Generate()

		return ii, nil
	})
	BeforeAll(s.Setup)
	AfterAll(s.TearDown)

	var id string

	It("creates and updates an IOU", func() {
		id = evmiou.CreateIOU(s.II, 10, "approver1")
		evmiou.CheckState(s.II, "borrower", id, 10)
		evmiou.CheckState(s.II, "lender", id, 10)

		evmiou.UpdateIOU(s.II, id, 5, "approver2")
		evmiou.CheckState(s.II, "borrower", id, 5)
		evmiou.CheckState(s.II, "lender", id, 5)
	})

	It("refuses an update that raises the amount", func() {
		_, err := s.II.Client("borrower").CallView("update", common.JSONMarshall(&views.UpdateIOUView{
			ID: id, Amount: 20, Lender: "lender", Approver: "approver1",
		}))
		Expect(err).To(HaveOccurred())
		evmiou.CheckState(s.II, "borrower", id, 5)
	})

	It("refuses a create whose lender FSC signature doesn't match", func() {
		_, err := s.II.Client("borrower").CallView("create", common.JSONMarshall(&views.CreateIOUView{
			Amount: 10, Lender: "lender", Approver: "approver1", ForgeLenderID: true,
		}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid lender FSC signature"))
	})

	It("reverts an update missing a signature", func() {
		p, ok := s.II.NWOCtx.PlatformByName(evm.TopologyName).(*evm.Platform)
		Expect(ok).To(BeTrue(), "expected an evm platform")
		ctx := context.Background()
		funder := p.Funder()

		iouID, err := client.HexToHash(id)
		Expect(err).NotTo(HaveOccurred())
		ret, err := p.Client().Call(ctx, p.Contract(), contract.EncodeGet([32]byte(iouID)), client.BlockTagLatest)
		Expect(err).NotTo(HaveOccurred())
		stored, err := contract.DecodeGet(ret)
		Expect(err).NotTo(HaveOccurred())

		// Valid borrower and lender signatures, so only the missing approver's makes it revert.
		u := contract.Update{ID: [32]byte(iouID), Amount: stored.Amount - 1, Version: stored.Version}
		digest := eip712.Digest(contract.Domain(funder.ChainID, p.Contract()), u.HashStruct())
		data := contract.EncodeUpdate(u, sign(s.II, "borrower", digest), sign(s.II, "lender", digest), nil)

		// Submitter.Send estimates gas, and the node refuses to estimate a call that reverts, so
		// the transaction is built with a fixed gas limit to get it mined.
		from := eip712.PubKeyToAddress(funder.Key.PubKey())
		nonce, err := p.Client().PendingNonceAt(ctx, from)
		Expect(err).NotTo(HaveOccurred())
		fees, err := p.Client().SuggestGasFees(ctx)
		Expect(err).NotTo(HaveOccurred())
		contractAddr := p.Contract()
		raw, err := client.SignTx(&client.DynamicFeeTx{
			ChainID:              funder.ChainID,
			Nonce:                nonce,
			MaxPriorityFeePerGas: fees.MaxPriorityFeePerGas,
			MaxFeePerGas:         fees.MaxFeePerGas,
			Gas:                  500_000,
			To:                   &contractAddr,
			Data:                 data,
		}, funder.Key)
		Expect(err).NotTo(HaveOccurred())
		h, err := p.Client().SendRawTransaction(ctx, raw)
		Expect(err).NotTo(HaveOccurred())

		_, err = chain.WaitReceipt(ctx, p.Client(), h, time.Second, time.Minute)
		Expect(err).To(MatchError(chain.ErrReverted))
		evmiou.CheckState(s.II, "borrower", id, stored.Amount)
	})
})

// sign signs digest with the EVM key the evm platform generated for the FSC node named name.
func sign(ii *integration.Infrastructure, name string, digest [32]byte) []byte {
	key, err := iou.LoadKey(filepath.Join(ii.NWOCtx.RootDir(), "evm", "keys", name+".key"))
	Expect(err).NotTo(HaveOccurred())
	sig, err := eip712.NewSigner(key).Sign(digest)
	Expect(err).NotTo(HaveOccurred())

	return sig
}
