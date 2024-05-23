/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/pkg/errors"
)

type Read struct {
	Key string
	Raw []byte
}

type ResultsIterator interface {
	// Next returns the next item in the result set. The `QueryResult` is expected to be nil when
	// the iterator gets exhausted
	Next() (*Read, error)
	// Close releases resources occupied by the iterator
	Close()
}

type VersionedRead = driver.VersionedRead

type VersionedResultsIterator = driver.VersionedResultsIterator

type WriteTransaction interface {
	// SetState sets the given value for the given namespace, key, and version
	SetState(namespace, key string, value []byte, block, txnum uint64) error
	// Commit commits the changes since BeginUpdate
	Commit() error
	// Discard discanrds the changes since BeginUpdate
	Discard() error
}

type TransactionalVersionedPersistence interface {
	VersionedPersistence

	NewWriteTransaction() (WriteTransaction, error)
}

type SQLError = error

var (
	// UniqueKeyViolation happens when we try to insert a record with a conflicting unique key (e.g. replicas)
	UniqueKeyViolation = errors.New("unique key violation")
	// DeadlockDetected happens when two transactions are taking place at the same time and interact with the same rows
	DeadlockDetected = errors.New("deadlock detected")
)

// SQLErrorWrapper transforms the different errors returned by various SQL implementations into an SQLError that is common
type SQLErrorWrapper interface {
	WrapError(error) error
}

// VersionedPersistence models a versioned key-value storage place
type VersionedPersistence interface {
	// SetState sets the given value for the given namespace, key, and version
	SetState(namespace, key string, value []byte, block, txnum uint64) error
	// GetState gets the value and version for given namespace and key
	GetState(namespace, key string) ([]byte, uint64, uint64, error)
	// DeleteState deletes the given namespace and key
	DeleteState(namespace, key string) error
	// GetStateMetadata gets the metadata and version for given namespace and key
	GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error)
	// SetStateMetadata sets the given metadata for the given namespace, key, and version
	SetStateMetadata(namespace, key string, metadata map[string][]byte, block, txnum uint64) error
	// GetStateRangeScanIterator returns an iterator that contains all the key-values between given key ranges.
	// startKey is included in the results and endKey is excluded. An empty startKey refers to the first available key
	// and an empty endKey refers to the last available key. For scanning all the keys, both the startKey and the endKey
	// can be supplied as empty strings. However, a full scan should be used judiciously for performance reasons.
	// The returned VersionedResultsIterator contains results of type *VersionedRead.
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (VersionedResultsIterator, error)
	// GetStateSetIterator returns an iterator that contains all the values for the passed keys.
	// The order is not respected.
	GetStateSetIterator(ns string, keys ...string) (VersionedResultsIterator, error)
	// Close closes this persistence instance
	Close() error
	// BeginUpdate starts the session
	BeginUpdate() error
	// Commit commits the changes since BeginUpdate
	Commit() error
	// Discard discanrds the changes since BeginUpdate
	Discard() error
}

// Persistence models a key-value storage place
type Persistence interface {
	// SetState sets the given value for the given namespace and key
	SetState(namespace, key string, value []byte) error
	// GetState gets the value for given namespace and key
	GetState(namespace, key string) ([]byte, error)
	// DeleteState deletes the given namespace and key
	DeleteState(namespace, key string) error
	// GetStateRangeScanIterator returns an iterator that contains all the key-values between given key ranges.
	// startKey is included in the results and endKey is excluded. An empty startKey refers to the first available key
	// and an empty endKey refers to the last available key. For scanning all the keys, both the startKey and the endKey
	// can be supplied as empty strings. However, a full scan should be used judiciously for performance reasons.
	// The returned ResultsIterator contains results of type *Read.
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ResultsIterator, error)
	// GetStateSetIterator returns an iterator that contains all the values for the passed keys.
	// The order is not respected.
	GetStateSetIterator(ns string, keys ...string) (ResultsIterator, error)
	// Close closes this persistence instance
	Close() error
	// BeginUpdate starts the session
	BeginUpdate() error
	// Commit commits the changes since BeginUpdate
	Commit() error
	// Discard discards the changes since BeginUpdate
	Discard() error
}

// Config provides access to the underlying configuration
type Config interface {
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes the value corresponding to the passed key and unmarshals it into the passed structure
	UnmarshalKey(key string, rawVal interface{}) error
}

type Driver interface {
	// NewTransactionalVersionedPersistence returns a new TransactionalVersionedPersistence for the passed data source and config
	NewTransactionalVersionedPersistence(dataSourceName string, config Config) (TransactionalVersionedPersistence, error)
	// NewVersioned returns a new VersionedPersistence for the passed data source and config
	NewVersioned(dataSourceName string, config Config) (VersionedPersistence, error)
	// New returns a new Persistence for the passed data source and config
	New(dataSourceName string, config Config) (Persistence, error)
}

type (
	ColumnKey       = string
	TriggerCallback func(Operation, map[ColumnKey]string)
	Operation       int
)

const (
	Unknown Operation = iota
	Delete
	Insert
	Update
)

type notifier interface {
	// Subscribe registers a listener for when a value is inserted/updated/deleted in the given table
	Subscribe(callback TriggerCallback) error
	// UnsubscribeAll removes all registered listeners for the given table
	UnsubscribeAll() error
}

type UnversionedNotifier interface {
	Persistence
	notifier
}
type VersionedNotifier interface {
	VersionedPersistence
	notifier
}
