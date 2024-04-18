/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
)

// FinalityListener is the interface that must be implemented to receive transaction status change notifications
type FinalityListener = driver.FinalityListener

// Committer models the committer service
type Committer struct {
	c           driver.Committer
	subscribers *events.Subscribers
}

func NewCommitter(c driver.Committer) *Committer {
	return &Committer{c: c, subscribers: events.NewSubscribers()}
}

// AddFinalityListener registers a listener for transaction status for the passed transaction id.
// If the status is already valid or invalid, the listener is called immediately.
// When the listener is invoked, then it is also removed.
// If the transaction id is empty, the listener will be called on status changes of any transaction.
// In this case, the listener is not removed
func (c *Committer) AddFinalityListener(txID string, listener FinalityListener) error {
	return c.c.AddFinalityListener(txID, listener)
}

// RemoveFinalityListener unregisters the passed listener.
func (c *Committer) RemoveFinalityListener(txID string, listener FinalityListener) error {
	return c.c.RemoveFinalityListener(txID, listener)
}
