/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type (
	TxValidationStatus = driver.TxValidationStatus[ValidationCode]
	QueryExecutor      = driver.QueryExecutor
)

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
