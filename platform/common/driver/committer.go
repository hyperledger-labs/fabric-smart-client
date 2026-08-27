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
	// WARNING: OnStatus MUST observe ctx.Done() and return promptly. An implementation
	// that blocks indefinitely -- on a full channel, a stalled store call, a contended
	// lock -- costs the driver that invoked it for as long as it runs, and nothing can
	// force a return: honoring cancellation is the implementation's responsibility.
	// Do slow or blocking work by handing it to your own queue and returning, not by
	// blocking here.
	//
	// What blocking costs depends on the driver, so assume the stricter of the two:
	//
	//   - fabricx (platform/fabricx/core/finality) invokes OnStatus from a pool of
	//     handlerWorkers and passes a ctx carrying a handlerTimeout deadline. A blocked
	//     callback holds one slot; once all are held, finality notifications stop being
	//     delivered until one frees.
	//   - the generic committer (platform/common/core/generic/committer,
	//     platform/fabric/core/generic/events) drains its event queue with
	//     eventQueueWorkers, but its parallel listener paths spawn one goroutine per
	//     listener and the ctx it passes carries no deadline of its own.
	//
	// So a callback that never returns can stall notification delivery on one driver and
	// leak a goroutine on the other.
	OnStatus(ctx context.Context, txID TxID, status V, statusMessage string)
}
