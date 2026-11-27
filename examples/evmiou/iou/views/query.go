/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/contract"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// QueryView returns the amount of the IOU with ID, a 0x-prefixed hex id, read from the contract
// at the latest block.
type QueryView struct {
	ID string
}

// Call reads the IOU and returns its amount as a uint64.
func (q *QueryView) Call(viewCtx view.Context) (any, error) {
	svc, err := iou.GetService(viewCtx)
	if err != nil {
		return nil, err
	}
	id, err := client.HexToHash(q.ID)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid iou id [%s]", q.ID)
	}
	stored, err := getIOU(viewCtx, svc, id)
	if err != nil {
		return nil, err
	}
	if stored.Version == 0 {
		return nil, errors.Errorf("iou [%s] does not exist", q.ID)
	}

	return stored.Amount, nil
}

// QueryViewFactory builds a QueryView from its JSON input.
type QueryViewFactory struct{}

// NewView unmarshals in into a QueryView.
func (*QueryViewFactory) NewView(in []byte) (view.View, error) {
	v := &QueryView{}
	if err := json.Unmarshal(in, v); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling the query input")
	}

	return v, nil
}

// getIOU reads the IOU stored at id through get(id) at the latest block.
func getIOU(viewCtx view.Context, svc *iou.Service, id [32]byte) (contract.IOU, error) {
	ret, err := svc.Client.Call(viewCtx.Context(), svc.Contract, contract.EncodeGet(id), client.BlockTagLatest)
	if err != nil {
		return contract.IOU{}, errors.Wrapf(err, "failed reading iou [%x]", id)
	}

	return contract.DecodeGet(ret)
}
