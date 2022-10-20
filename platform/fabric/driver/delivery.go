/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "context"

// DeliveryCallback is a callback function used to process a transaction.
// Return true, if the scan should finish.
type DeliveryCallback func(tx ProcessedTransaction) (bool, error)

// Delivery gives access to Fabric channel delivery
type Delivery interface {
	// StartDelivery starts the delivery process
	StartDelivery(ctx context.Context) error

	// Scan iterates over all transactions in block starting from the block containing the passed transaction id.
	// If txID is empty, the iterations starts from the first block.
	// On each transaction, the callback function is invoked.
	Scan(ctx context.Context, txID string, callback DeliveryCallback) error
}
