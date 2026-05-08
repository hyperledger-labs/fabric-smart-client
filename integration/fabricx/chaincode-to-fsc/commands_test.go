/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincodetofsc_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	chaincodetofsc "github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc/states"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

const defaultViewTimeout = 2 * time.Minute

// initEndorser tells the endorser FSC node to start processing namespace
// commit events. Mirrors iou's InitApprover.
func initEndorser(ii *integration.Infrastructure) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	_, err := ii.Client(chaincodetofsc.EndorserNode).CallViewWithContext(ctx, "init", nil)
	Expect(err).NotTo(HaveOccurred())
}

func initLedger(ii *integration.Infrastructure) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	_, err := ii.Client(chaincodetofsc.IssuerNode).CallViewWithContext(ctx, "init_ledger",
		common.JSONMarshall(views.InitParams{
			Endorser: ii.Identity(chaincodetofsc.EndorserNode),
			Auditor:  ii.Identity(chaincodetofsc.AuditorNode),
		}))
	Expect(err).NotTo(HaveOccurred(), "init_ledger should succeed")
}

func createAsset(ii *integration.Infrastructure, initiator string, a *states.Asset) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	_, err := ii.Client(initiator).CallViewWithContext(ctx, "create_asset",
		common.JSONMarshall(views.CreateParams{
			ID:             a.ID,
			Color:          a.Color,
			Size:           a.Size,
			Owner:          a.Owner,
			AppraisedValue: a.AppraisedValue,
			Endorser:       ii.Identity(chaincodetofsc.EndorserNode),
			Auditor:        ii.Identity(chaincodetofsc.AuditorNode),
		}))
	Expect(err).NotTo(HaveOccurred(), "create_asset %s", a.ID)
}

func createAssetExpectFail(ii *integration.Infrastructure, initiator string, a *states.Asset) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	_, err := ii.Client(initiator).CallViewWithContext(ctx, "create_asset",
		common.JSONMarshall(views.CreateParams{
			ID: a.ID, Color: a.Color, Size: a.Size, Owner: a.Owner, AppraisedValue: a.AppraisedValue,
			Endorser: ii.Identity(chaincodetofsc.EndorserNode),
			Auditor:  ii.Identity(chaincodetofsc.AuditorNode),
		}))
	Expect(err).To(HaveOccurred(), "create_asset %s should have failed", a.ID)
}

func readAsset(ii *integration.Infrastructure, reader, id string) *states.Asset {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	res, err := ii.Client(reader).CallViewWithContext(ctx, "read_asset",
		common.JSONMarshall(views.ReadParams{ID: id}))
	Expect(err).NotTo(HaveOccurred(), "read_asset %s", id)
	raw, ok := res.([]byte)
	Expect(ok).To(BeTrue())
	out := &states.Asset{}
	Expect(json.Unmarshal(raw, out)).To(Succeed())
	return out
}

func readAssetExpectFail(ii *integration.Infrastructure, reader, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	_, err := ii.Client(reader).CallViewWithContext(ctx, "read_asset",
		common.JSONMarshall(views.ReadParams{ID: id}))
	Expect(err).To(HaveOccurred())
}

func assetExists(ii *integration.Infrastructure, reader, id string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	res, err := ii.Client(reader).CallViewWithContext(ctx, "asset_exists",
		common.JSONMarshall(views.ReadParams{ID: id}))
	Expect(err).NotTo(HaveOccurred())
	raw, ok := res.([]byte)
	Expect(ok).To(BeTrue())
	var b bool
	Expect(json.Unmarshal(raw, &b)).To(Succeed())
	return b
}

func updateAsset(ii *integration.Infrastructure, initiator string, a *states.Asset) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	_, err := ii.Client(initiator).CallViewWithContext(ctx, "update_asset",
		common.JSONMarshall(views.UpdateParams{
			ID: a.ID, Color: a.Color, Size: a.Size, Owner: a.Owner, AppraisedValue: a.AppraisedValue,
			Endorser: ii.Identity(chaincodetofsc.EndorserNode),
			Auditor:  ii.Identity(chaincodetofsc.AuditorNode),
		}))
	Expect(err).NotTo(HaveOccurred(), "update_asset %s", a.ID)
}

func deleteAsset(ii *integration.Infrastructure, initiator, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	_, err := ii.Client(initiator).CallViewWithContext(ctx, "delete_asset",
		common.JSONMarshall(views.DeleteParams{
			ID:       id,
			Endorser: ii.Identity(chaincodetofsc.EndorserNode),
			Auditor:  ii.Identity(chaincodetofsc.AuditorNode),
		}))
	Expect(err).NotTo(HaveOccurred(), "delete_asset %s", id)
}

func transferAsset(ii *integration.Infrastructure, initiator, id, newOwnerLabel, newOwnerNode string) string {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	res, err := ii.Client(initiator).CallViewWithContext(ctx, "transfer_asset",
		common.JSONMarshall(views.TransferParams{
			ID:            id,
			NewOwnerLabel: newOwnerLabel,
			NewOwner:      ii.Identity(newOwnerNode),
			Endorser:      ii.Identity(chaincodetofsc.EndorserNode),
			Auditor:       ii.Identity(chaincodetofsc.AuditorNode),
		}))
	Expect(err).NotTo(HaveOccurred(), "transfer_asset %s -> %s", id, newOwnerLabel)
	return common.JSONUnmarshalString(res)
}

func transferAssetExpectFail(ii *integration.Infrastructure, initiator, id, newOwnerLabel, newOwnerNode string) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	_, err := ii.Client(initiator).CallViewWithContext(ctx, "transfer_asset",
		common.JSONMarshall(views.TransferParams{
			ID:            id,
			NewOwnerLabel: newOwnerLabel,
			NewOwner:      ii.Identity(newOwnerNode),
			Endorser:      ii.Identity(chaincodetofsc.EndorserNode),
			Auditor:       ii.Identity(chaincodetofsc.AuditorNode),
		}))
	Expect(err).To(HaveOccurred())
}

func getAllAssets(ii *integration.Infrastructure, reader string, ids []string) []*states.Asset {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()
	res, err := ii.Client(reader).CallViewWithContext(ctx, "get_all_assets",
		common.JSONMarshall(views.GetAllParams{IDs: ids}))
	Expect(err).NotTo(HaveOccurred())
	raw, ok := res.([]byte)
	Expect(ok).To(BeTrue())
	var out []*states.Asset
	Expect(json.Unmarshal(raw, &out)).To(Succeed())
	return out
}
