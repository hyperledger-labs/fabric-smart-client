/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type FinalityView struct {
	Network   string
	Channel   string
	TxID      string
	Endpoints []view.Identity
	Timeout   time.Duration
}

func (f *FinalityView) Call(ctx view.Context) (interface{}, error) {
	fns := fabric.GetFabricNetworkService(ctx, f.Network)
	if fns == nil {
		return nil, errors.Errorf("fabric network service [%s] not found", f.Network)
	}
	ch, err := fns.Channel(f.Channel)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", f.Network, f.Channel)
	}
	if len(f.Endpoints) != 0 {
		return nil, ch.Finality().IsFinalForParties(f.TxID, f.Endpoints...)
	}
	c := ctx.Context()
	if f.Timeout != 0 {
		var cancel context.CancelFunc
		c, cancel = context.WithTimeout(c, f.Timeout)
		defer cancel()
	}
	return f.TxID, ch.Finality().IsFinal(c, f.TxID)
}

func NewFinalityView(tx *Transaction) *FinalityView {
	return &FinalityView{
		Network: tx.Network(),
		Channel: tx.Channel(),
		TxID:    tx.ID(),
	}
}

// NewFinalityWithTimeoutView runs the finality view for the passed transaction and Timeout
func NewFinalityWithTimeoutView(tx *Transaction, timeout time.Duration) *FinalityView {
	return &FinalityView{
		Network: tx.Network(),
		Channel: tx.Channel(),
		TxID:    tx.ID(),
		Timeout: timeout,
	}
}

func NewFinalityFromView(tx *Transaction, endpoints ...view.Identity) *FinalityView {
	return &FinalityView{
		Network:   tx.Network(),
		Channel:   tx.Channel(),
		TxID:      tx.ID(),
		Endpoints: endpoints,
	}
}

func NewFinalityFor(network, channel, txID string, timeout time.Duration) *FinalityView {
	return &FinalityView{
		Network: network,
		Channel: channel,
		TxID:    txID,
		Timeout: timeout,
	}
}
