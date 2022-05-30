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

// TxStatusChangeListener is the interface that must be implemented to receive transaction status change notifications
type TxStatusChangeListener interface {
	// OnStatusChange is called when the status of a transaction changes
	OnStatusChange(txID string, status int) error
}

// Committer models the committer service
type Committer interface {
	// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed id
	SubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error

	// UnsubscribeTxStatusChanges unregisters a listener for transaction status changes for the passed id
	UnsubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error
}
