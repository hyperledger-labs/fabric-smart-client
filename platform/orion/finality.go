/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
)

type Finality struct {
	finality driver.Finality
}

func (c *Finality) IsFinal(ctx context.Context, txID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return c.finality.IsFinal(ctx, txID)
}
