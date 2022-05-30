/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type TransactionStatusChanged struct {
	ThisTopic string
	TxID      string
	VC        ValidationCode
}

func (t *TransactionStatusChanged) Topic() string {
	return t.ThisTopic
}

func (t *TransactionStatusChanged) Message() interface{} {
	return t
}

// TxStatusListener is a callback function that is called when a transaction
// status changes.
// If a timeout is reached, the function is called with timeout set to true.
type TxStatusListener func(txID string, status ValidationCode) error

// Committer models the committer service
type Committer interface {
	// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed id
	SubscribeTxStatusChanges(txID string, listener TxStatusListener) error

	// UnsubscribeTxStatusChanges unregisters a listener for transaction status changes for the passed id
	UnsubscribeTxStatusChanges(txID string, listener TxStatusListener) error
}
