/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "github.com/hyperledger/fabric-protos-go/common"

// ValidationCode of transaction
type ValidationCode int

const (
	_               ValidationCode = iota
	Valid                          // Transaction is valid and committed
	Invalid                        // Transaction is invalid and has been discarded
	Busy                           // Transaction does not yet have a validity state
	Unknown                        // Transaction is unknown
	HasDependencies                // Transaction is unknown but has known dependencies
)

// TransactionStatusChanged is sent when the status of a transaction changes
type TransactionStatusChanged struct {
	ThisTopic string
	TxID      string
	VC        ValidationCode
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
	OnStatusChange(txID string, status int) error
}

// Committer models the committer service
type Committer interface {
	// ProcessNamespace registers namespaces that will be committed even if the rwset is not known
	ProcessNamespace(nss ...string) error

	// AddProcessor(ns string, processor Processor) error

	// Status returns a validation code this committer bind to the passed transaction id, plus
	// a list of dependant transaction ids if they exist.
	Status(txid string) (ValidationCode, []string, error)

	// DiscardTx discards the transaction with the passed id and all its dependencies, if they exists.
	DiscardTx(txid string) error

	// CommitTX commits the transaction with the passed id and all its dependencies, if they exists.
	// Depending on tx's status, CommitTX does the following:
	// Tx is Unknown, CommitTx does nothing and returns no error.
	// Tx is HasDependencies, CommitTx proceeds with the multi-shard private transaction commit protocol.
	// Tx is Valid, CommitTx does nothing and returns an error.
	// Tx is Invalid, CommitTx does nothing and returns an error.
	// Tx is Busy, if Tx is a multi-shard private transaction then CommitTx proceeds with the multi-shard private transaction commit protocol,
	// otherwise, CommitTx commits the transaction.
	CommitTX(txid string, block uint64, indexInBloc int, envelope *common.Envelope) error

	// CommitConfig commits the passed configuration envelope.
	CommitConfig(blockNumber uint64, raw []byte, envelope *common.Envelope) error

	// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed transaction id.
	// If the transaction id is empty, the listener will be called for all transactions.
	SubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error

	// UnsubscribeTxStatusChanges unregisters a listener for transaction status changes for the passed transaction id.
	// If the transaction id is empty, the listener will be called for all transactions.
	UnsubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error
}
