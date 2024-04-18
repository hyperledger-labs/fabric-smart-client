/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
)

// Committer models the committer service
type Committer interface {
	// AddFinalityListener registers a listener for transaction status for the passed transaction id.
	// If the status is already valid or invalid, the listener is called immediately.
	// When the listener is invoked, then it is also removed.
	// If the transaction id is empty, the listener will be called on status changes of any transaction.
	// In this case, the listener is not removed
	AddFinalityListener(txID string, listener driver.FinalityListener) error

	// RemoveFinalityListener unregisters the passed listener.
	RemoveFinalityListener(txID string, listener driver.FinalityListener) error

	AddTransactionFilter(filter driver.TransactionFilter) error
}

func NewCommitter(c driver.Committer) Committer {
	return c
}
