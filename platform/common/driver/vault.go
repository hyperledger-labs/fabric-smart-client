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

type VersionedValue struct {
	Raw     RawValue
	Version RawVersion
}

type VersionedMetadataValue struct {
	Version  RawVersion
	Metadata Metadata
}

type TxStatusCode int32

const (
	Unknown TxStatusCode = iota
	Valid
	Invalid
	Busy
)

type TxStatus struct {
	TxID    TxID
	Code    TxStatusCode
	Message string
}

type TxStatusIterator = collections.Iterator[*TxStatus]
type TxStateIterator = collections.Iterator[*VersionedRead]

type VersionedResultsIterator = collections.Iterator[*VersionedRead]

type QueryExecutor interface {
	GetState(namespace Namespace, key PKey) (*VersionedRead, error)
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

type MetaWrites map[Namespace]map[PKey]VersionedMetadataValue

type Writes map[Namespace]map[PKey]VersionedValue

// VaultLock represents a lock over a transaction or the whole vault
type VaultLock interface {
	// Release releases the locked resources
	Release() error
}

type VaultStore interface {
	// AcquireTxIDRLock acquires a read lock on a specific transaction.
	// While holding this lock, other routines:
	// - cannot update the transaction states or statuses of the locked transactions
	// - can read the locked transactions
	AcquireTxIDRLock(txID TxID) (VaultLock, error)

	// AcquireGlobalLock acquires a global exclusive read lock on the vault.
	// While holding this lock, other routines:
	// - cannot acquire this read lock (exclusive)
	// - cannot update any transaction states or statuses
	// - can read any transaction
	AcquireGlobalLock() (VaultLock, error)

	// GetStateMetadata returns the metadata for the given specific namespace - key pair
	GetStateMetadata(namespace Namespace, key PKey) (Metadata, RawVersion, error)

	// GetState returns the state for the given specific namespace - key pair
	GetState(namespace Namespace, key PKey) (*VersionedRead, error)

	// GetStates returns the states for the given specific namespace - key pairs
	GetStates(namespace Namespace, keys ...PKey) (TxStateIterator, error)

	// GetStateRange returns the states for the given specific namespace - key range
	GetStateRange(namespace Namespace, startKey, endKey PKey) (TxStateIterator, error)

	// GetAllStates returns all states for a given namespace. Only used for testing purposes.
	GetAllStates(namespace Namespace) (TxStateIterator, error)

	// Store stores atomically the transaction statuses, writes and metadata writes
	Store(txIDs []TxID, writes Writes, metaWrites MetaWrites) error

	// GetLast returns the status of the latest non-pending transaction
	GetLast() (*TxStatus, error)

	// GetTxStatus returns the status of the given transaction
	GetTxStatus(txID TxID) (*TxStatus, error)

	// GetTxStatuses returns the statuses of the given transactions
	GetTxStatuses(txIDs ...TxID) (TxStatusIterator, error)

	// GetAllTxStatuses returns the statuses of the all transactions in the vault
	GetAllTxStatuses() (TxStatusIterator, error)

	// SetStatuses sets the status and message for the given transactions
	SetStatuses(code TxStatusCode, message string, txIDs ...TxID) error

	// Close closes the vault store
	Close() error
}
