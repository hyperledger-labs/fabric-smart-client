/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"encoding/json"
	"time"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type Finality struct {
	TxID    driver2.TxID
	Network driver2.Network
	Channel driver2.Channel
}

type finalityView struct {
	*Finality
	timeout time.Duration
}

func (f *finalityView) Call(ctx view.Context) (interface{}, error) {
	return ctx.RunView(&FinalityView{
		Finality:    f.Finality,
		timeout:     f.timeout,
		fnsProvider: utils.MustGet(fabric.GetNetworkServiceProvider(ctx)),
	})
}

type EndorserFinalityViewFactory struct{}

func (f *EndorserFinalityViewFactory) NewView(in []byte) (view.View, error) {
	v := &finalityView{Finality: &Finality{}}
	if err := json.Unmarshal(in, v.Finality); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling input")
	}
	return v, nil
}

type FinalityView struct {
	*Finality
	timeout time.Duration

	fnsProvider *fabric.NetworkServiceProvider
}

func (f *FinalityView) Call(ctx view.Context) (interface{}, error) {
	fns, err := f.fnsProvider.FabricNetworkService(f.Network)
	if err != nil {
		return nil, errors.WithMessagef(err, "fabric network service [%s] not found", f.Network)
	}
	ch, err := fns.Channel(f.Channel)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", f.Network, f.Channel)
	}
	c := ctx.Context()
	if f.timeout != 0 {
		var cancel context.CancelFunc
		c, cancel = context.WithTimeout(c, f.timeout)
		defer cancel()
	}
	return nil, ch.Finality().IsFinal(c, f.TxID)
}

func NewFinalityView(tx *Transaction) *finalityView {
	return &finalityView{Finality: &Finality{TxID: tx.ID(), Network: tx.Network(), Channel: tx.Channel()}}
}

// NewFinalityWithTimeoutView runs the finality view for the passed transaction and timeout
func NewFinalityWithTimeoutView(tx *Transaction, timeout time.Duration) *finalityView {
	return &finalityView{Finality: &Finality{TxID: tx.ID(), Network: tx.Network(), Channel: tx.Channel()}, timeout: timeout}
}

func NewFinalityViewFactory(fnsProvider *fabric.NetworkServiceProvider) *FinalityViewFactory {
	return &FinalityViewFactory{fnsProvider: fnsProvider}
}

type FinalityViewFactory struct {
	fnsProvider *fabric.NetworkServiceProvider
}

func (f *FinalityViewFactory) NewView(in []byte) (view.View, error) {
	v := &FinalityView{Finality: &Finality{}, fnsProvider: f.fnsProvider}
	if err := json.Unmarshal(in, v.Finality); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling input")
	}
	return v, nil
}

func (f *FinalityViewFactory) New(tx *Transaction) *FinalityView {
	return f.NewWithTimeout(tx, 0)
}

func (f *FinalityViewFactory) NewWithTimeout(tx *Transaction, timeout time.Duration) *FinalityView {
	return &FinalityView{
		Finality: &Finality{
			TxID:    tx.ID(),
			Network: tx.Network(),
			Channel: tx.Channel(),
		},
		timeout:     timeout,
		fnsProvider: f.fnsProvider,
	}
}
