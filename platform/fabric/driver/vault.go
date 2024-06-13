/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type (
	TxValidationStatus = driver2.TxValidationStatus[ValidationCode]
	BlockNum           = driver2.BlockNum
	TxNum              = driver2.TxNum
)

type Vault interface {
	driver2.Vault[ValidationCode]

	// InspectRWSet returns an ephemeral RWSet for this ledger whose content is unmarshalled
	// from the passed bytes.
	// If namespaces is not empty, the returned RWSet will be filtered by the passed namespaces
	InspectRWSet(rwset []byte, namespaces ...string) (RWSet, error)
	RWSExists(id string) bool
	Match(id string, results []byte) error
	Close() error
}
