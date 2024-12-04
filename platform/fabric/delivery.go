/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
)

type DeliveryCallback func(tx *ProcessedTransaction) (bool, error)

type BlockCallback func(context.Context, *common.Block) (bool, error)

// Delivery models the Fabric's delivery service
type Delivery struct {
	delivery driver.Delivery
}

func (d *Delivery) ScanBlock(ctx context.Context, callback BlockCallback) error {
	return d.delivery.ScanBlock(ctx, func(ctx context.Context, block *common.Block) (bool, error) {
		return callback(ctx, block)
	})
}

// Scan iterates over all transactions in block starting from the block containing the passed transaction id.
// If txID is empty, the iterations starts from the first block.
// On each transaction, the callback function is invoked.
func (d *Delivery) Scan(ctx context.Context, txID string, callback DeliveryCallback) error {
	return d.delivery.Scan(ctx, txID, func(tx driver.ProcessedTransaction) (bool, error) {
		return callback(&ProcessedTransaction{
			pt: tx,
		})
	})
}
