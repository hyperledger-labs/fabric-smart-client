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
	// OnStatus is called when the status of a transaction changes
	OnStatus(txID string, status driver.ValidationCode, statusMessage string)
}

type Committer struct {
	committer   driver.Committer
	subscribers *events.Subscribers
}

func NewCommitter(ch driver.Channel) *Committer {
	return &Committer{committer: ch.Committer(), subscribers: events.NewSubscribers()}
}

// ProcessNamespace registers namespaces that will be committed even if the rwset is not known
func (c *Committer) ProcessNamespace(nss ...string) error {
	return c.committer.ProcessNamespace(nss...)
}

// Status returns a validation code this committer bind to the passed transaction id, plus
// a list of dependant transaction ids if they exist.
func (c *Committer) Status(txID string) (ValidationCode, string, error) {
	vc, message, err := c.committer.Status(txID)
	return ValidationCode(vc), message, err
}

func (c *Committer) AddStatusReporter(sr driver.StatusReporter) error {
	return c.committer.AddStatusReporter(sr)
}

// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed transaction id.
// If the transaction id is empty, the listener will be called for all transactions.
func (c *Committer) SubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error {
	return c.committer.AddFinalityListener(txID, listener)
}

// UnsubscribeTxStatusChanges unregisters a listener for transaction status changes for the passed transaction id.
// If the transaction id is empty, the listener will be called for all transactions.
func (c *Committer) UnsubscribeTxStatusChanges(txID string, listener TxStatusChangeListener) error {
	return c.committer.RemoveFinalityStatus(txID, listener)
}
