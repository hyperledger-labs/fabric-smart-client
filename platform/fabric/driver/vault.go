/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

const (
	FromStorage      = driver2.FromStorage
	FromIntermediate = driver2.FromIntermediate
	FromBoth         = driver2.FromBoth
)

type TxValidationStatus = driver2.TxValidationStatus[ValidationCode]

type Vault interface {
	driver2.Vault[ValidationCode]

	// GetEphemeralRWSet returns an ephemeral RWSet for this ledger whose content is unmarshalled
	// from the passed bytes.
	// If namespaces is not empty, the returned RWSet will be filtered by the passed namespaces
	GetEphemeralRWSet(rwset []byte, namespaces ...string) (RWSet, error)
	RWSExists(id string) bool
	Match(id string, results []byte) error
	Close() error
}
