/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type DeliveryCallback func(tx *ProcessedTransaction) (bool, error)

// Delivery models the Fabric's delivery service
type Delivery struct {
	ch *Channel
}

// Scan iterates over all transactions in block starting from the block containing the passed transaction id.
// If txID is empty, the iterations starts from the first block.
// On each transaction, the callback function is invoked.
func (d *Delivery) Scan(ctx context.Context, txID string, callback DeliveryCallback) error {
	return d.ch.ch.Scan(ctx, txID, func(tx driver.ProcessedTransaction) (bool, error) {
		return callback(&ProcessedTransaction{
			pt: tx,
		})
	})
}
