/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// GetAllParams gives the GetAllAssetsView a list of candidate asset IDs to
// resolve. See the GetAllAssetsView godoc for why this list is passed in
// rather than discovered via a range scan.
type GetAllParams struct {
	IDs []string
}

// GetAllAssetsView is the FSC analogue of:
//
//	func (s *SmartContract) GetAllAssets(ctx) ([]*Asset, error)
//
// Migration sharp edge: classical chaincode used GetStateByRange("", "")
// to walk every key in the chaincode's namespace. Fabric-X's Query Service
// does NOT support range queries — this is documented in the platform
// code at platform/fabricx/core/vault/vault.go where GetStateRange returns
// "GetStateRange not supported by VaultX QueryService".
//
// Three migration patterns close this gap:
//
//  1. Explicit-ID-list (this implementation). Callers pass the IDs they want.
//     Simple; no on-chain bookkeeping; suitable when the caller already
//     tracks issued asset IDs (e.g. a registry app).
//
//  2. Index-key pattern. Every Create / Delete also writes to a dedicated
//     index key whose value is the JSON-encoded list of live asset IDs.
//     GetAllAssets reads the index, then calls GetStates on each ID.
//     Adds one extra output per state-changing transaction. The endorser
//     must check the index update is internally consistent.
//
//  3. Off-chain index. An FSC node subscribes to the namespace's commit
//     events (via finality listeners on every committed transaction) and
//     keeps a local index. GetAllAssets reads the local index. Stronger
//     decoupling; weaker consistency guarantees on cold start.
//
// The tutorial chapter "When the migration is harder" walks through all
// three. This file implements pattern 1 because it surfaces the issue
// honestly without inventing on-chain machinery the chaincode did not have.
type GetAllAssetsView struct {
	GetAllParams
}

func (g *GetAllAssetsView) Call(viewCtx view.Context) (interface{}, error) {
	network, ch, err := fabric.GetDefaultChannel(viewCtx)
	assert.NoError(err)
	qs, err := queryservice.GetQueryService(viewCtx, network.Name(), ch.Name())
	assert.NoError(err)

	// Bulk fetch via GetStates — one round trip rather than len(g.IDs).
	keys := make([]driver.PKey, len(g.IDs))
	for i, id := range g.IDs {
		keys[i] = id
	}
	out, err := qs.GetStates(map[driver.Namespace][]driver.PKey{Namespace: keys})
	assert.NoError(err)

	assets := make([]*states.Asset, 0, len(g.IDs))
	for _, id := range g.IDs {
		val, ok := out[Namespace][id]
		if !ok || val.Raw == nil {
			continue
		}
		a := &states.Asset{}
		if err := json.Unmarshal(val.Raw, a); err == nil {
			assets = append(assets, a)
		}
	}
	return assets, nil
}

type GetAllAssetsViewFactory struct{}

func (*GetAllAssetsViewFactory) NewView(in []byte) (view.View, error) {
	f := &GetAllAssetsView{}
	assert.NoError(json.Unmarshal(in, &f.GetAllParams), "GetAll bad input")
	return f, nil
}
