/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package driver

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

type VersionedResultsIterator interface {
	// Next returns the next item in the result set. The `QueryResult` is expected to be nil when
	// the iterator gets exhausted
	Next() (*VersionedRead, error)
	// Close releases resources occupied by the iterator
	Close()
}

type VersionedPersistence interface {
	SetState(namespace, key string, value []byte, block, txnum uint64) error
	GetState(namespace, key string) ([]byte, uint64, uint64, error)
	DeleteState(namespace, key string) error
	GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error)
	SetStateMetadata(namespace, key string, metadata map[string][]byte, block, txnum uint64) error
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (VersionedResultsIterator, error)
	Close() error
	BeginUpdate() error
	Commit() error
	Discard() error
}

type Persistence interface {
	SetState(namespace, key string, value []byte) error
	GetState(namespace, key string) ([]byte, error)
	DeleteState(namespace, key string) error
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ResultsIterator, error)
	Close() error
	BeginUpdate() error
	Commit() error
	Discard() error
}
