/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// FinalityListener is the interface that must be implemented to receive transaction status notifications
type FinalityListener interface {
	// OnStatus is called when the status of a transaction changes
	OnStatus(txID string, status driver.ValidationCode, statusMessage string)
}

type Committer struct {
	committer driver.Committer
}

func NewCommitter(ch driver.Channel) *Committer {
	return &Committer{committer: ch.Committer()}
}

// ProcessNamespace registers namespaces that will be committed even if the rwset is not known
func (c *Committer) ProcessNamespace(nss ...string) error {
	return c.committer.ProcessNamespace(nss...)
}

// Status returns a validation code this committer bind to the passed transaction id, plus
// a list of dependant transaction ids if they exist.
func (c *Committer) Status(txID string) (ValidationCode, string, error) {
	return c.committer.Status(txID)
}

// AddFinalityListener registers a listener for transaction status for the passed transaction id.
// If the status is already valid or invalid, the listener is called immediately.
// When the listener is invoked, then it is also removed.
// If the transaction id is empty, the listener will be called on status changes of any transaction.
// In this case, the listener is not removed
func (c *Committer) AddFinalityListener(txID string, listener FinalityListener) error {
	return c.committer.AddFinalityListener(txID, listener)
}

// RemoveFinalityListener unregisters the passed listener.
func (c *Committer) RemoveFinalityListener(txID string, listener FinalityListener) error {
	return c.committer.RemoveFinalityListener(txID, listener)
}
