/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
)

type handler struct {
	nsp *fabric.NetworkServiceProvider
}

func NewHandler(fns *fabric.NetworkServiceProvider) finality.Handler {
	return &handler{nsp: fns}
}

func (f *handler) IsFinal(ctx context.Context, network, channel, txID string) error {
	if f.nsp == nil {
		return fmt.Errorf("cannot find fabric network provider")
	}
	fns, err := f.nsp.FabricNetworkService(network)
	if fns == nil || err != nil {
		return fmt.Errorf("cannot find fabric network [%s]: %w", network, err)
	}

	ch, err := fns.Channel(channel)
	if err != nil {
		return fmt.Errorf("failed to get channel [%s] on fabric network [%s]: %w", channel, network, err)
	}
	return ch.Finality().IsFinal(ctx, txID)
}
