/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
)

// TxStatusChangeListener is the interface that must be implemented to receive transaction status change notifications
type TxStatusChangeListener interface {
	// OnStatusChange is called when the status of a transaction changes
	OnStatus(txID string, status driver.ValidationCode, statusMessage string)
}

// Committer models the committer service
type Committer struct {
	c           driver.Committer
	subscribers *events.Subscribers
}

func NewCommitter(c driver.Committer) *Committer {
	return &Committer{c: c, subscribers: events.NewSubscribers()}
}

// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed transaction id.
// If the transaction id is empty, the listener will be called for all transactions.
func (c *Committer) SubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error {
	return c.c.AddFinalityListener(txID, listener)
}

// UnsubscribeTxStatusChanges unregisters a listener for transaction status changes for the passed transaction id.
// If the transaction id is empty, the listener will be called for all transactions.
func (c *Committer) UnsubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error {
	return c.c.RemoveFinalityListener(txID, listener)
}
