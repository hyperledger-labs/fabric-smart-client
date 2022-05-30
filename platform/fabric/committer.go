/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"sync"
)

// TxStatusListener is a callback function that is called when a transaction
// status changes.
// If a timeout is reached, the function is called with timeout set to true.
type TxStatusListener func(txID string, status ValidationCode) error

type Committer struct {
	ch          driver.Channel
	subscribers *sync.Map
}

func NewCommitter(ch driver.Channel) *Committer {
	return &Committer{ch: ch, subscribers: &sync.Map{}}
}

// ProcessNamespace registers namespaces that will be committed even if the rwset is not known
func (c *Committer) ProcessNamespace(nss ...string) error {
	return c.ch.ProcessNamespace(nss...)
}

// Status returns a validation code this committer bind to the passed transaction id, plus
// a list of dependant transaction ids if they exist.
func (c *Committer) Status(txid string) (ValidationCode, []string, error) {
	vc, deps, err := c.ch.Status(txid)
	return ValidationCode(vc), deps, err
}

// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed id
func (c *Committer) SubscribeTxStatusChanges(txID string, listener TxStatusListener) error {
	l := func(txID string, status driver.ValidationCode) error {
		return listener(txID, ValidationCode(status))
	}
	c.subscribers.Store(txID, l)
	return c.ch.SubscribeTxStatusChanges(txID, l)
}

// UnsubscribeTxStatusChanges unregisters a listener for transaction status changes for the passed id
func (c *Committer) UnsubscribeTxStatusChanges(txID string, listener TxStatusListener) error {
	l, ok := c.subscribers.Load(listener)
	if !ok {
		return nil
	}
	l1, ok := l.(driver.TxStatusListener)
	if !ok {
		return nil
	}
	c.subscribers.Delete(listener)
	return c.ch.UnsubscribeTxStatusChanges(txID, l1)
}
