/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "github.com/hyperledger-labs/fabric-smart-client/platform/view"

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

type VersionedRead struct {
	Key          string
	Raw          []byte
	Block        uint64
	IndexInBlock int
}

func (v *VersionedRead) K() string {
	return v.Key
}

func (v *VersionedRead) V() []byte {
	return v.Raw
}

type VersionedResultsIterator interface {
	// Next returns the next item in the result set. The `QueryResult` is expected to be nil when
	// the iterator gets exhausted
	Next() (*VersionedRead, error)
	// Close releases resources occupied by the iterator
	Close()
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
	//add load(identity) driver
	// NewVersioned returns a new VersionedPersistence for the passed data source and config
	NewVersioned(sp view.ServiceProvider, dataSourceName string, config Config) (VersionedPersistence, error)
	// New returns a new Persistence for the passed data source and config
	New(sp view.ServiceProvider, dataSourceName string, config Config) (Persistence, error)
}
