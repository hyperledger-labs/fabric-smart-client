/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "context"

// TransactionFilter is used to filter unknown transactions.
// If the filter accepts, the transaction is processed by the commit pipeline anyway.
type TransactionFilter interface {
	Accept(txID TxID, env []byte) (bool, error)
}

// FinalityListener is the interface that must be implemented to receive transaction status notifications
type FinalityListener[V comparable] interface {
	// OnStatus is called when the status of a transaction changes, or it is already valid or invalid.
	//
	// WARNING: OnStatus MUST observe ctx.Done() and return promptly. Nothing can force a
	// return, so honoring cancellation is the implementation's responsibility: hand slow
	// or blocking work to your own queue and return, rather than blocking here.
	//
	// Every driver runs OnStatus on a bounded worker pool, so a callback that blocks
	// does not merely cost its own goroutine: it holds a worker, and enough stuck
	// callbacks stop finality delivery altogether.
	//
	//   - fabricx (platform/fabricx/core/finality) invokes it from handlerWorkers
	//     goroutines, on a context carrying a handlerTimeout deadline.
	//   - the generic committer (platform/common/core/generic/committer) drains its
	//     event queue with eventQueueWorkers goroutines and calls listeners
	//     synchronously from them.
	OnStatus(ctx context.Context, txID TxID, status V, statusMessage string)
}
