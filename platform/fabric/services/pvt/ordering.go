/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvt

import (
	"encoding/base64"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

// OrderingView stores the hash of the passed transaction into a Fabric namespace
type OrderingView struct {
	pvtNamespace string
	tx           *fabric.Transaction
}

func NewOrderingView(tx *fabric.Transaction) *OrderingView {
	return &OrderingView{tx: tx}
}

// Call executes the view
func (o *OrderingView) Call(ctx view.Context) (interface{}, error) {
	tx := o.tx

	// store envelope
	fns := fabric.GetFabricNetworkService(ctx, tx.Network())
	if fns == nil {
		return nil, errors.Errorf("cannot find fabric network service [%s]", tx.Network())
	}
	ch, err := fns.Channel(tx.Channel())
	if err != nil {
		return nil, errors.WithMessagef(err, "cannot find fabric channle [%s:%s]", tx.Network(), tx.Channel())
	}
	txRaw, err := tx.Bytes()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to marshal transaction [%s], [%s:%s]", tx.ID(), tx.Network(), tx.Channel())
	}
	if err := ch.TransactionService().StoreTransaction(tx.ID(), txRaw); err != nil {
		return nil, errors.Wrapf(err, "failed to store transaction's envelope [%s]", tx.ID())
	}

	// Store hash
	hash := hash.Hashable(txRaw).String()
	logger.Debugf("store hash in chaincode [%s:%s]", tx.ID(), hash)
	txID, _, err := chaincode.NewInvokeView(
		o.pvtNamespace,
		"store",
		base64.StdEncoding.EncodeToString([]byte(tx.ID())),
		hash,
	).WithChannel(
		tx.Channel(),
	).WithNetwork(
		tx.Network(),
	).WithSignerIdentity(
		fns.LocalMembership().AnonymousIdentity(),
	).Invoke(ctx)
	if err != nil {
		return nil, err
	}
	return txID, nil
}

// SetNamespace sets the fabric namespace to use to store the hash of the transactions
func (o *OrderingView) SetNamespace(ns string) *OrderingView {
	o.pvtNamespace = ns
	return o
}
