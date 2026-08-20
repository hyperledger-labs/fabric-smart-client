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
	// WARNING: OnStatus MUST observe ctx.Done() and return promptly. Drivers invoke
	// it with bounded concurrency, so an implementation that blocks indefinitely --
	// on a full channel, a stalled store call, a contended lock -- holds one of a
	// fixed number of slots for as long as it runs. Once every slot is held, no
	// further finality notification can be delivered, and further ones are dropped
	// with a warning. The ctx carries a deadline, but nothing can force a return:
	// honoring it is the implementation's responsibility.
	//
	// Do slow or blocking work by handing it to your own queue and returning, not
	// by blocking here. Both drivers dispatch from a bounded pool: the generic
	// committer's eventQueueWorkers (platform/common/core/generic/committer) and
	// the fabricx notification service's handlerWorkers
	// (platform/fabricx/core/finality).
	OnStatus(ctx context.Context, txID TxID, status V, statusMessage string)
}
