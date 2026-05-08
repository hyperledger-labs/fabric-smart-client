/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// AssetExistsView is the FSC analogue of:
//
//	func (s *SmartContract) AssetExists(ctx, id) (bool, error)
//
// Like ReadAssetView this is query-only. The single difference is that we
// don't unmarshal the value — we just return whether the Query Service
// produced a non-nil result.
type AssetExistsView struct {
	ReadParams // reuse the same {ID string} input shape
}

func (e *AssetExistsView) Call(viewCtx view.Context) (interface{}, error) {
	network, ch, err := fabric.GetDefaultChannel(viewCtx)
	assert.NoError(err)
	qs, err := queryservice.GetQueryService(viewCtx, network.Name(), ch.Name())
	assert.NoError(err)

	val, err := qs.GetState(Namespace, e.ID)
	assert.NoError(err)
	return val != nil, nil
}

type AssetExistsViewFactory struct{}

func (*AssetExistsViewFactory) NewView(in []byte) (view.View, error) {
	f := &AssetExistsView{}
	assert.NoError(json.Unmarshal(in, &f.ReadParams), "Exists bad input")
	return f, nil
}
