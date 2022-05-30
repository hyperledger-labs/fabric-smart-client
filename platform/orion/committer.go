/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"sync"
)

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
type Committer struct {
	c           driver.Committer
	subscribers *sync.Map
}

func NewCommitter(c driver.Committer) *Committer {
	return &Committer{c: c, subscribers: &sync.Map{}}
}

// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed id
func (c *Committer) SubscribeTxStatusChanges(txID string, listener TxStatusListener) error {
	l := func(txID string, status driver.ValidationCode) error {
		return listener(txID, ValidationCode(status))
	}
	c.subscribers.Store(txID, l)
	return c.c.SubscribeTxStatusChanges(txID, l)
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
	return c.c.UnsubscribeTxStatusChanges(txID, l1)
}
