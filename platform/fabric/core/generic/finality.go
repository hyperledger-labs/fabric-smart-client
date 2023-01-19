/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func (c *Channel) IsFinal(ctx context.Context, txID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return c.finality.IsFinal(ctx, txID)
}

func (c *Channel) IsFinalForParties(txID string, parties ...view.Identity) error {
	return c.finality.IsFinalForParties(txID, parties...)
}
