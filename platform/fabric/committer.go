/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"

// TxStatusListener is a callback function that is called when a transaction
// status changes.
// If a timeout is reached, the function is called with timeout set to true.
type TxStatusListener func(txID string, status ValidationCode, timeout bool) error

type Committer struct {
	ch driver.Channel
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

func (c *Committer) TxStatusListen(txID string, listener TxStatusListener) error {
	return c.ch.TxStatusListen(txID, func(txID string, status driver.ValidationCode, timeout bool) error {
		return listener(txID, ValidationCode(status), timeout)
	})
}
