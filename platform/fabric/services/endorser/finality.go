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

type finalityView struct {
	tx      *Transaction
	timeout time.Duration
}

func (f *finalityView) Call(ctx view.Context) (interface{}, error) {
	fns, err := fabric.GetFabricNetworkService(ctx, f.tx.Network())
	if err != nil {
		return nil, errors.WithMessagef(err, "fabric network service [%s] not found", f.tx.Network())
	}
	ch, err := fns.Channel(f.tx.Channel())
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", f.tx.Network(), f.tx.Channel())
	}
	c := ctx.Context()
	if f.timeout != 0 {
		var cancel context.CancelFunc
		c, cancel = context.WithTimeout(c, f.timeout)
		defer cancel()
	}
	return nil, ch.Finality().IsFinal(c, f.tx.ID())
}

func NewFinalityView(tx *Transaction) *finalityView {
	return &finalityView{tx: tx}
}

// NewFinalityWithTimeoutView runs the finality view for the passed transaction and timeout
func NewFinalityWithTimeoutView(tx *Transaction, timeout time.Duration) *finalityView {
	return &finalityView{tx: tx, timeout: timeout}
}
