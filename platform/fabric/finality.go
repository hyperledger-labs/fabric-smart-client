/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"golang.org/x/net/context"
)

type Finality struct {
	ch driver.Channel
}

func (c *Finality) IsFinal(ctx context.Context, txID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return c.ch.IsFinal(ctx, txID)
}

func (c *Finality) IsFinalForParties(txID string, parties ...view.Identity) error {
	return c.ch.IsFinalForParties(txID, parties...)
}
