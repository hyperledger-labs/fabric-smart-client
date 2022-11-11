/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
)

// TxStatusChangeListener is the interface that must be implemented to receive transaction status change notifications
type TxStatusChangeListener interface {
	// OnStatusChange is called when the status of a transaction changes
	OnStatusChange(txID string, status int) error
}

type Committer struct {
	ch          driver.Channel
	subscribers *events.Subscribers
}

func NewCommitter(ch driver.Channel) *Committer {
	return &Committer{ch: ch, subscribers: events.NewSubscribers()}
}

// Status returns a validation code this committer bind to the passed transaction id, plus
// a list of dependant transaction ids if they exist.
func (c *Committer) Status(txid string) (ValidationCode, []string, error) {
	vc, deps, err := c.ch.Status(txid)
	return ValidationCode(vc), deps, err
}

// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed transaction id.
// If the transaction id is empty, the listener will be called for all transactions.
func (c *Committer) SubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error {
	return c.ch.SubscribeTxStatusChanges(txID, listener)
}

// UnsubscribeTxStatusChanges unregisters a listener for transaction status changes for the passed transaction id.
// If the transaction id is empty, the listener will be called for all transactions.
func (c *Committer) UnsubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error {
	return c.ch.UnsubscribeTxStatusChanges(txID, listener)
}
