/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

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

type TransactionFilter interface {
	Accept(txID string, env []byte) (bool, error)
}

// TxStatusChangeListener is the interface that must be implemented to receive transaction status change notifications
type TxStatusChangeListener interface {
	// OnStatusChange is called when the status of a transaction changes
	OnStatusChange(txID string, status int, statusMessage string) error
}

// Committer models the committer service
type Committer interface {
	// AddTransactionFilter adds a new transaction filter to this commit pipeline.
	// The transaction filter is used to check if an unknown transaction needs to be processed anyway
	AddTransactionFilter(tf TransactionFilter) error

	// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed transaction id.
	// If the transaction id is empty, the listener will be called for all transactions.
	SubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error

	// UnsubscribeTxStatusChanges unregisters a listener for transaction status changes for the passed transaction id.
	// If the transaction id is empty, the listener will be called for all transactions.
	UnsubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error
}
