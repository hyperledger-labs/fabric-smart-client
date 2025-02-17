/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type (
	TxValidationStatus = driver.TxValidationStatus[ValidationCode]
	QueryExecutor      = driver.QueryExecutor
	BlockNum           = driver.BlockNum
	TxID               = driver.TxID
	TxNum              = driver.TxNum
	TxStatus           = driver.TxStatus
)

type Vault interface {
	driver.Vault[ValidationCode]

	// InspectRWSet returns an ephemeral RWSet for this ledger whose content is unmarshalled
	// from the passed bytes.
	// If namespaces is not empty, the returned RWSet will be filtered by the passed namespaces
	InspectRWSet(ctx context.Context, rwset []byte, namespaces ...driver.Namespace) (RWSet, error)
	RWSExists(ctx context.Context, id driver.TxID) bool
	Match(ctx context.Context, id driver.TxID, results []byte) error
	Close() error
}

type VaultStore interface {
	GetState(ctx context.Context, namespace driver.Namespace, key driver.PKey) (*driver.VaultRead, error)
	GetStateRange(ctx context.Context, namespace driver.Namespace, startKey, endKey driver.PKey) (driver.TxStateIterator, error)
	GetLast(ctx context.Context) (*TxStatus, error)
}
