/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// FinalityListener is the interface that must be implemented to receive transaction status notifications
type FinalityListener = driver.FinalityListener

// TransactionFilter is used to filter unknown transactions.
// If the filter accepts, the transaction is processed by the commit pipeline anyway.
type TransactionFilter = driver.TransactionFilter

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

func (c *Committer) DiscardNamespace(nss ...string) error {
	return c.committer.DiscardNamespace(nss...)
}

// Status returns a validation code this committer bind to the passed transaction id, plus
// a list of dependant transaction ids if they exist.
func (c *Committer) Status(txID string) (ValidationCode, string, error) {
	return c.committer.Status(txID)
}

// AddFinalityListener registers a listener for transaction status for the passed transaction id.
// If the status is already valid or invalid, the listener is called immediately.
// When the listener is invoked, then it is also removed.
// The transaction id must not be empty.
func (c *Committer) AddFinalityListener(txID string, listener FinalityListener) error {
	return c.committer.AddFinalityListener(txID, listener)
}

// RemoveFinalityListener unregisters the passed listener.
func (c *Committer) RemoveFinalityListener(txID string, listener FinalityListener) error {
	return c.committer.RemoveFinalityListener(txID, listener)
}

// AddTransactionFilter adds a new transaction filter to this commit pipeline.
// The transaction filter is used to check if an unknown transaction needs to be processed anyway
func (c *Committer) AddTransactionFilter(tf TransactionFilter) error {
	return c.committer.AddTransactionFilter(tf)
}
