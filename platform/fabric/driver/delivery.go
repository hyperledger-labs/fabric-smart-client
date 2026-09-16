/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
)

// DeliveryCallback is a callback function used to process a transaction.
// Return true, if the scan should finish.
type DeliveryCallback func(tx ProcessedTransaction) (bool, error)

// BlockCallback is the callback function prototype to alert the rest of the stack about the availability of a new block.
// It returns a boolean to signal that delivery should stop, and an error to
// signal that processing the block failed.
//
// An error stops delivery for that channel. Whatever could be retried is expected
// to have been retried by the callback already - the committer, for one, retries a
// transient commit failure on the block internally - so an error returned here is
// final and is not attempted again. To end delivery without reporting a failure,
// return true instead.
type BlockCallback func(context.Context, *common.Block) (bool, error)

// Delivery gives access to Fabric channel delivery
type Delivery interface {
	// Start starts the delivery process
	Start(ctx context.Context) error

	// ScanBlock iterates over all blocks.
	// On each block, the callback function is invoked.
	ScanBlock(ctx context.Context, callback BlockCallback) error

	// ScanBlockFrom iterates over all blocks starting from the block with the passed number.
	// On each block, the callback function is invoked.
	ScanBlockFrom(ctx context.Context, block BlockNum, callback BlockCallback) error

	// Scan iterates over all transactions in block starting from the block containing the passed transaction id.
	// If txID is empty, the iterations starts from the first block.
	// On each transaction, the callback function is invoked.
	Scan(ctx context.Context, txID TxID, callback DeliveryCallback) error

	// ScanFromBlock iterates over all transactions in block starting from the block with the passed number.
	// On each transaction, the callback function is invoked.
	ScanFromBlock(ctx context.Context, block BlockNum, callback DeliveryCallback) error
}
