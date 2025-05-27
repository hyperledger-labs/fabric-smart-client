/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import "math"

// Batch lazily batches the elements of an iterator into groups of {{batchSize}} sized batches
// Each element of the output Iterator will now be a batch, i.e. a slice of the initial elements
func Batch[V any](it Iterator[*V], batchSize uint32) Iterator[*[]*V] {
	batchCap := batchSize
	if batchSize == 0 {
		batchSize = math.MaxUint32
	}
	return &batched[V]{Iterator: it, batchSize: int(batchSize), batchCap: int(batchCap)}
}

type batched[V any] struct {
	Iterator[*V]
	batchSize int
	batchCap  int
}

func (b *batched[V]) Next() (*[]*V, error) {
	batch := make([]*V, 0, b.batchCap)

	for item, err := b.Iterator.Next(); ; item, err = b.Iterator.Next() {
		if err != nil {
			return nil, err
		}
		if item == nil && len(batch) == 0 {
			return nil, nil
		}
		if item == nil && len(batch) > 0 {
			return &batch, nil
		}
		batch = append(batch, item)
		if len(batch) >= b.batchSize {
			return &batch, nil
		}
	}
}
