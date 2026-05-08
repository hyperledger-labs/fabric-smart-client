/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ReadParams identifies the asset to read.
type ReadParams struct {
	ID string
}

// ReadAssetView is the FSC analogue of:
//
//	func (s *SmartContract) ReadAsset(ctx, id) (*Asset, error)
//
// This is a query-only view: it does NOT build a transaction, does NOT
// collect endorsements, does NOT submit to the orderer. The Fabric-X
// Committer's Query Service is the read-side microservice and the FSC
// platform exposes it through queryservice.GetQueryService.
//
// Migration mapping:
//
//	chaincode               FSC-on-Fabric-X
//	---------------------   --------------------------------
//	ctx.GetStub().GetState  qs.GetState(namespace, key)
type ReadAssetView struct {
	ReadParams
}

func (r *ReadAssetView) Call(viewCtx view.Context) (interface{}, error) {
	network, ch, err := fabric.GetDefaultChannel(viewCtx)
	assert.NoError(err, "Read: get default channel")
	qs, err := queryservice.GetQueryService(viewCtx, network.Name(), ch.Name())
	assert.NoError(err, "Read: get query service")

	val, err := qs.GetState(Namespace, r.ID)
	assert.NoError(err, "Read: query state")
	if val == nil {
		// Match the chaincode's error shape:
		//   "the asset %s does not exist"
		// so a migration-time client gets a recognisable message.
		return nil, errors.Errorf("the asset %s does not exist", r.ID)
	}

	asset := &states.Asset{}
	assert.NoError(json.Unmarshal(val.Raw, asset), "Read: unmarshal asset")
	return asset, nil
}

type ReadAssetViewFactory struct{}

func (*ReadAssetViewFactory) NewView(in []byte) (view.View, error) {
	f := &ReadAssetView{}
	assert.NoError(json.Unmarshal(in, &f.ReadParams), "Read bad input")
	return f, nil
}
