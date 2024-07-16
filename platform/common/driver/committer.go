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
	// OnStatus is called when the status of a transaction changes, or it is already valid or invalid
	OnStatus(ctx context.Context, txID TxID, status V, statusMessage string)
}
