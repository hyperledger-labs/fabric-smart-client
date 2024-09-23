/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
)

type (
	PKey       = string
	MKey       = string
	RawValue   = []byte
	Metadata   = map[MKey][]byte
	RawVersion = []byte
)

type VersionedRead struct {
	Key     PKey
	Raw     RawValue
	Version RawVersion
}

type VersionedMetadata struct {
	Metadata Metadata
	Version  RawVersion
}

type VersionedMetadataValue struct {
	Block    BlockNum
	TxNum    TxNum
	Metadata Metadata
}

type VersionedResultsIterator = collections.Iterator[*VersionedRead]

type QueryExecutor interface {
	GetState(namespace Namespace, key PKey) (RawValue, error)
	GetStateMetadata(namespace Namespace, key PKey) (Metadata, RawVersion, error)
	GetStateRangeScanIterator(namespace Namespace, startKey PKey, endKey PKey) (VersionedResultsIterator, error)
	Done()
}

type TxValidationStatus[V comparable] struct {
	TxID           TxID
	ValidationCode V
	Message        string
}

// Vault models a key value store that can be updated by committing rwsets
type Vault[V comparable] interface {
	// NewQueryExecutor gives handle to a query executor.
	// A client can obtain more than one 'QueryExecutor's for parallel execution.
	// Any synchronization should be performed at the implementation level if required
	NewQueryExecutor() (QueryExecutor, error)

	// NewRWSet returns a RWSet for this ledger.
	// A client may obtain more than one such simulator; they are made unique
	// by way of the supplied txid
	NewRWSet(txID TxID) (RWSet, error)

	// GetRWSet returns a RWSet for this ledger whose content is unmarshalled
	// from the passed bytes.
	// A client may obtain more than one such simulator; they are made unique
	// by way of the supplied txid
	GetRWSet(txID TxID, rwset []byte) (RWSet, error)

	SetDiscarded(txID TxID, message string) error

	Status(txID TxID) (V, string, error)

	Statuses(txIDs ...TxID) ([]TxValidationStatus[V], error)

	// DiscardTx discards the transaction with the given transaction id.
	// If no error occurs, invoking Status on the same transaction id will return the Invalid flag.
	DiscardTx(txID TxID, message string) error

	CommitTX(ctx context.Context, txID TxID, block BlockNum, index TxNum) error
}
