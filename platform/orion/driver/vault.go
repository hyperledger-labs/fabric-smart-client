/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"

const (
	FromStorage      = driver.FromStorage
	FromIntermediate = driver.FromIntermediate
	FromBoth         = driver.FromBoth
)

type TxValidationStatus = driver.TxValidationStatus[ValidationCode]

// Vault models a key value store that can be updated by committing rwsets
type Vault interface {
	driver.Vault[ValidationCode]
	AddStatusReporter(sr StatusReporter) error
	GetLastTxID() (string, error)
}
