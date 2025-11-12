/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Finality struct {
	TxID    cdriver.TxID
	Network cdriver.Network
	Channel cdriver.Channel
}

type finalityView struct {
	*Finality
	timeout time.Duration
}

func (f *finalityView) Call(ctx view.Context) (interface{}, error) {
	fns, err := fabric.GetFabricNetworkService(ctx, f.Network)
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

type FinalityViewFactory struct{}

func (p *FinalityViewFactory) NewView(in []byte) (view.View, error) {
	f := &finalityView{Finality: &Finality{}}
	if err := json.Unmarshal(in, f.Finality); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling input")
	}
	return f, nil
}
