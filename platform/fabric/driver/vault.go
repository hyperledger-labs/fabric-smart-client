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

type Vault interface {
	driver.Vault[ValidationCode]

	// GetEphemeralRWSet returns an ephemeral RWSet for this ledger whose content is unmarshalled
	// from the passed bytes.
	// If namespaces is not empty, the returned RWSet will be filtered by the passed namespaces
	GetEphemeralRWSet(rwset []byte, namespaces ...string) (RWSet, error)
	RWSExists(id string) bool
	Match(id string, results []byte) error
	Close() error
}
