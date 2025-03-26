/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type orderingView struct {
	tx       *Transaction
	finality bool
	timeout  time.Duration
}

func (o *orderingView) Call(ctx view.Context) (interface{}, error) {
	return ctx.RunView(&OrderingView{
		tx:           o.tx,
		finality:     o.finality,
		timeout:      o.timeout,
		fnsProvider:  utils.MustGet(fabric.GetNetworkServiceProvider(ctx)),
		finalityView: NewFinalityViewFactory(utils.MustGet(fabric.GetNetworkServiceProvider(ctx))),
	})
}

type OrderingView struct {
	tx       *Transaction
	finality bool
	timeout  time.Duration

	fnsProvider  *fabric.NetworkServiceProvider
	finalityView *FinalityViewFactory
}

func (o *OrderingView) Call(ctx view.Context) (interface{}, error) {
	fns, err := o.fnsProvider.FabricNetworkService(o.tx.Network())
	if err != nil {
		return nil, errors.WithMessagef(err, "fabric network service [%s] not found", o.tx.Network())
	}
	tx := o.tx
	if err := fns.Ordering().Broadcast(ctx.Context(), tx.Transaction); err != nil {
		return nil, errors.WithMessagef(err, "failed broadcasting to [%s:%s]", o.tx.Network(), o.tx.Channel())
	}
	if o.finality {
		return ctx.RunView(o.finalityView.NewWithTimeout(tx, o.timeout))
	}
	return tx, nil
}

func NewOrderingAndFinalityView(tx *Transaction) *orderingView {
	return &orderingView{tx: tx, finality: true}
}

func NewOrderingAndFinalityWithTimeoutView(tx *Transaction, timeout time.Duration) *orderingView {
	return &orderingView{tx: tx, finality: true, timeout: timeout}
}

func NewOrderingView(tx *Transaction) *orderingView {
	return &orderingView{tx: tx, finality: false}
}

type OrderingAndFinalityViewFactory struct {
	fnsProvider  *fabric.NetworkServiceProvider
	finalityView *FinalityViewFactory
}

func NewOrderingAndFinalityViewFactory(
	fnsProvider *fabric.NetworkServiceProvider,
	finalityView *FinalityViewFactory,
) *OrderingAndFinalityViewFactory {
	return &OrderingAndFinalityViewFactory{
		fnsProvider:  fnsProvider,
		finalityView: finalityView,
	}
}

func (f *OrderingAndFinalityViewFactory) New(tx *Transaction, finality bool) *OrderingView {
	return f.NewWithTimeout(tx, finality, 0)
}

func (f *OrderingAndFinalityViewFactory) NewWithTimeout(tx *Transaction, finality bool, timeout time.Duration) *OrderingView {
	return &OrderingView{
		tx:           tx,
		finality:     finality,
		timeout:      timeout,
		fnsProvider:  f.fnsProvider,
		finalityView: f.finalityView,
	}
}
