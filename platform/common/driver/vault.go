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

type VaultRead struct {
	Key     PKey
	Raw     RawValue
	Version RawVersion
}

type UnversionedRead struct {
	Key PKey
	Raw RawValue
}
type UnversionedValue = RawValue

type VaultValue struct {
	Raw     RawValue
	Version RawVersion
}

type VaultMetadataValue struct {
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
type TxStateIterator = collections.Iterator[*VaultRead]

type VersionedResultsIterator = collections.Iterator[*VaultRead]

type QueryExecutor interface {
	GetState(ctx context.Context, namespace Namespace, key PKey) (*VaultRead, error)
	GetStateMetadata(ctx context.Context, namespace Namespace, key PKey) (Metadata, RawVersion, error)
	GetStateRangeScanIterator(ctx context.Context, namespace Namespace, startKey PKey, endKey PKey) (VersionedResultsIterator, error)
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
	NewQueryExecutor(ctx context.Context) (QueryExecutor, error)

	// NewRWSet returns a RWSet for this ledger.
	// A client may obtain more than one such simulator; they are made unique
	// by way of the supplied txid
	NewRWSet(ctx context.Context, txID TxID) (RWSet, error)

	// GetRWSet returns a RWSet for this ledger whose content is unmarshalled
	// from the passed bytes.
	// A client may obtain more than one such simulator; they are made unique
	// by way of the supplied txid
	GetRWSet(ctx context.Context, txID TxID, rwset []byte) (RWSet, error)

	SetDiscarded(ctx context.Context, txID TxID, message string) error

	Status(ctx context.Context, txID TxID) (V, string, error)

	Statuses(ctx context.Context, txIDs ...TxID) ([]TxValidationStatus[V], error)

	// DiscardTx discards the transaction with the given transaction id.
	// If no error occurs, invoking Status on the same transaction id will return the Invalid flag.
	DiscardTx(ctx context.Context, txID TxID, message string) error

	CommitTX(ctx context.Context, txID TxID, block BlockNum, index TxNum) error
}

type MetaWrites map[Namespace]map[PKey]VaultMetadataValue

type Writes map[Namespace]map[PKey]VaultValue

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
	AcquireTxIDRLock(ctx context.Context, txID TxID) (VaultLock, error)

	// AcquireGlobalLock acquires a global exclusive read lock on the vault.
	// While holding this lock, other routines:
	// - cannot acquire this read lock (exclusive)
	// - cannot update any transaction states or statuses
	// - can read any transaction
	AcquireGlobalLock(ctx context.Context) (VaultLock, error)

	// GetStateMetadata returns the metadata for the given specific namespace - key pair
	GetStateMetadata(ctx context.Context, namespace Namespace, key PKey) (Metadata, RawVersion, error)

	// GetState returns the state for the given specific namespace - key pair
	GetState(ctx context.Context, namespace Namespace, key PKey) (*VaultRead, error)

	// GetStates returns the states for the given specific namespace - key pairs
	GetStates(ctx context.Context, namespace Namespace, keys ...PKey) (TxStateIterator, error)

	// GetStateRange returns the states for the given specific namespace - key range
	GetStateRange(ctx context.Context, namespace Namespace, startKey, endKey PKey) (TxStateIterator, error)

	// GetAllStates returns all states for a given namespace. Only used for testing purposes.
	GetAllStates(ctx context.Context, namespace Namespace) (TxStateIterator, error)

	// Store stores atomically the transaction statuses, writes and metadata writes
	Store(ctx context.Context, txIDs []TxID, writes Writes, metaWrites MetaWrites) error

	// GetLast returns the status of the latest non-pending transaction
	GetLast(ctx context.Context) (*TxStatus, error)

	// GetTxStatus returns the status of the given transaction
	GetTxStatus(ctx context.Context, txID TxID) (*TxStatus, error)

	// GetTxStatuses returns the statuses of the given transactions
	GetTxStatuses(ctx context.Context, txIDs ...TxID) (TxStatusIterator, error)

	// GetAllTxStatuses returns the statuses of the all transactions in the vault
	GetAllTxStatuses(ctx context.Context) (TxStatusIterator, error)

	// SetStatuses sets the status and message for the given transactions
	SetStatuses(ctx context.Context, code TxStatusCode, message string, txIDs ...TxID) error

	// Close closes the vault store
	Close() error
}
