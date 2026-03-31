/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

type baseIterator[k any] interface {
	// Next returns the next item in the result set. The `QueryResult` is expected to be nil when
	// the iterator gets exhausted
	Next() (k, error)
}

type Iterator[V any] interface {
	baseIterator[V]

	// Close releases resources occupied by the iterator
	Close()
}

type Transformer[A any, B any] func(A) (B, error)
