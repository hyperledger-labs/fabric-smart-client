/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type Finality struct {
	finality driver.Finality
}

func (c *Finality) IsFinal(ctx context.Context, txID string) error {
	return c.finality.IsFinal(ctx, txID)
}
