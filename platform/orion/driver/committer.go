/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
)

type StatusReporter interface {
	Status(txID string) (ValidationCode, string, []string, error)
}

// TransactionStatusChanged is the message sent when the status of a transaction changes
type TransactionStatusChanged struct {
	ThisTopic         string
	TxID              string
	VC                ValidationCode
	ValidationMessage string
}

// Topic returns the topic for the message
func (t *TransactionStatusChanged) Topic() string {
	return t.ThisTopic
}

// Message returns the message itself
func (t *TransactionStatusChanged) Message() interface{} {
	return t
}

type TransactionFilter = driver.TransactionFilter

// FinalityListener is the interface that must be implemented to receive transaction status change notifications
type FinalityListener = committer.FinalityListener[ValidationCode]

// Committer models the committer service
type Committer interface {
	// AddTransactionFilter adds a new transaction filter to this commit pipeline.
	// The transaction filter is used to check if an unknown transaction needs to be processed anyway
	AddTransactionFilter(tf TransactionFilter) error

	// AddFinalityListener registers a listener for transaction status for the passed transaction id.
	// If the status is already valid or invalid, the listener is called immediately.
	// When the listener is invoked, then it is also removed.
	// If the transaction id is empty, the listener will be called on status changes of any transaction.
	// In this case, the listener is not removed
	AddFinalityListener(txID string, listener FinalityListener) error

	// RemoveFinalityListener unregisters the passed listener.
	RemoveFinalityListener(txID string, listener FinalityListener) error
}
