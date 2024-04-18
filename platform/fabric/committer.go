/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type Committer interface {
	// ProcessNamespace registers namespaces that will be committed even if the rwset is not known
	ProcessNamespace(nss ...string) error
	// Status returns a validation code this committer bind to the passed transaction id, plus
	// a list of dependant transaction ids if they exist.
	Status(txID string) (driver.ValidationCode, string, error)
	AddTransactionFilter(sr driver.TransactionFilter) error
	// AddFinalityListener registers a listener for transaction status for the passed transaction id.
	// If the status is already valid or invalid, the listener is called immediately.
	// When the listener is invoked, then it is also removed.
	// If the transaction id is empty, the listener will be called on status changes of any transaction.
	// In this case, the listener is not removed
	AddFinalityListener(txID string, listener driver.FinalityListener) error
	// RemoveFinalityListener unregisters the passed listener.
	RemoveFinalityListener(txID string, listener driver.FinalityListener) error
}

func NewCommitter(ch driver.Channel) Committer {
	return ch.Committer()
}
