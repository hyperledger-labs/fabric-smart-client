/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

// ValidationCode of transaction
type ValidationCode int

const (
	_       ValidationCode = iota
	Valid                  // Transaction is valid and committed
	Invalid                // Transaction is invalid and has been discarded
	Busy                   // Transaction does not yet have a validity state
	Unknown                // Transaction is unknown
)

// TransactionStatusChanged is sent when the status of a transaction changes
type TransactionStatusChanged struct {
	ThisTopic         string
	TxID              string
	VC                ValidationCode
	ValidationMessage string
}

// Topic returns the topic for the transaction status change
func (t *TransactionStatusChanged) Topic() string {
	return t.ThisTopic
}

// Message returns the message for the transaction status change
func (t *TransactionStatusChanged) Message() interface{} {
	return t
}

// TxStatusChangeListener is the interface that must be implemented to receive transaction status change notifications
type TxStatusChangeListener interface {
	// OnStatusChange is called when the status of a transaction changes
	OnStatusChange(txID string, status int, statusMessage string) error
}

type TransactionFilter interface {
	Accept(txID string, env []byte) (bool, error)
}

// Committer models the committer service
type Committer interface {
	// ProcessNamespace registers namespaces that will be committed even if the rwset is not known
	ProcessNamespace(nss ...string) error

	// AddTransactionFilter adds a new transaction filter to this commit pipeline.
	// The transaction filter is used to check if an unknown transaction needs to be processed anyway
	AddTransactionFilter(tf TransactionFilter) error

	// Status returns a validation code this committer bind to the passed transaction id, plus
	// a list of dependant transaction ids if they exist.
	Status(txID string) (ValidationCode, string, error)

	// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed transaction id.
	// If the transaction id is empty, the listener will be called for all transactions.
	SubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error

	// UnsubscribeTxStatusChanges unregisters a listener for transaction status changes for the passed transaction id.
	// If the transaction id is empty, the listener will be called for all transactions.
	UnsubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error
}
