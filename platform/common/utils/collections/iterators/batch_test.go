/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

var testMatrix = []struct {
	name      string
	items     []any
	batchSize uint32
	expected  [][]any
}{
	{
		name:      "empty source",
		items:     []any{},
		batchSize: 10,
		expected:  [][]any{},
	},
	{
		name:      "evenly divides",
		items:     []any{1, 2, 3, 4},
		batchSize: 2,
		expected:  [][]any{{1, 2}, {3, 4}},
	},
	{
		name:      "trailing partial batch",
		items:     []any{1, 2, 3, 4},
		batchSize: 3,
		expected:  [][]any{{1, 2, 3}, {4}},
	},
	{
		name:      "batch size larger than source",
		items:     []any{1, 2, 3, 4},
		batchSize: 5,
		expected:  [][]any{{1, 2, 3, 4}},
	},
	{
		name:      "zero batch size means unbounded",
		items:     []any{1, 2, 3, 4},
		batchSize: 0,
		expected:  [][]any{{1, 2, 3, 4}},
	},
}

func TestBatchedIterator(t *testing.T) {
	t.Parallel()

	for _, testCase := range testMatrix {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			it := iterators.Slice(toPointerSlice(testCase.items))
			batched := iterators.Batch[any](it, testCase.batchSize)
			actual, err := iterators.ReadAllPointers(batched)
			require.NoError(t, err)
			require.Len(t, actual, len(testCase.expected))
			for i := range testCase.expected {
				require.Equal(t, toPointerSlice(testCase.expected[i]), *actual[i])
			}
		})
	}
}

func TestBatchedIteratorPropagatesError(t *testing.T) {
	t.Parallel()

	// the source fails while accumulating the second batch, after it has
	// already read one element (4) into it
	source := newFailing(ptrs(1, 2, 3, 4, 5), 4)
	batched := iterators.Batch[int](source, 3)

	first, err := batched.Next()
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, derefAll(*first))

	// the batch in progress when the source fails (the lone element 4) is
	// discarded, not returned alongside the error
	_, err = batched.Next()
	require.ErrorIs(t, err, errAt)
}

func derefAll[T any](ps []*T) []T {
	vs := make([]T, len(ps))
	for i, p := range ps {
		vs[i] = *p
	}
	return vs
}

func toPointerSlice[V any](vs []V) []*V {
	ps := make([]*V, len(vs))
	for i, v := range vs {
		ps[i] = &v
	}
	return ps
}
