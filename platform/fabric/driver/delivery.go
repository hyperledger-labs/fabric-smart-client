/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/common"
)

// DeliveryCallback is a callback function used to process a transaction.
// Return true, if the scan should finish.
type DeliveryCallback func(tx ProcessedTransaction) (bool, error)

// BlockCallback is the callback function prototype to alert the rest of the stack about the availability of a new block.
// The function returns two argument a boolean to signal if delivery should be stopped, and an error
// to signal an issue during the processing of the block.
// In case of an error, the same block is re-processed after a delay.
type BlockCallback func(context.Context, *common.Block) (bool, error)

// Delivery gives access to Fabric channel delivery
type Delivery interface {
	// Start starts the delivery process
	Start(ctx context.Context) error

	// ScanBlock iterates over all blocks.
	// On each block, the callback function is invoked.
	ScanBlock(ctx context.Context, callback BlockCallback) error

	// Scan iterates over all transactions in block starting from the block containing the passed transaction id.
	// If txID is empty, the iterations starts from the first block.
	// On each transaction, the callback function is invoked.
	Scan(ctx context.Context, txID string, callback DeliveryCallback) error
}
