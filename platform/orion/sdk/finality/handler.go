/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
)

func NewHandler(ons *orion.NetworkService) *handler {
	return &handler{ons: ons}
}

type handler struct {
	ons *orion.NetworkService
}

func (f *handler) IsFinal(ctx context.Context, network, channel, txID string) error {
	return f.ons.Finality().IsFinal(ctx, txID)
}
