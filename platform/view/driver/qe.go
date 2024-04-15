/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

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

type QueryExecutor interface {
	GetState(namespace string, key string) ([]byte, error)
	GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error)
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (VersionedResultsIterator, error)
	Done()
}
