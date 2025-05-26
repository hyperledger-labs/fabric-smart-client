/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	. "github.com/onsi/gomega"
)

var testMatrix = []struct {
	items     []any
	batchSize uint32
	expected  [][]any
}{
	{
		items:     []any{},
		batchSize: 10,
		expected:  [][]any{},
	},
	{
		items:     []any{1, 2, 3, 4},
		batchSize: 2,
		expected:  [][]any{{1, 2}, {3, 4}},
	},
	{
		items:     []any{1, 2, 3, 4},
		batchSize: 3,
		expected:  [][]any{{1, 2, 3}, {4}},
	},
	{
		items:     []any{1, 2, 3, 4},
		batchSize: 5,
		expected:  [][]any{{1, 2, 3, 4}},
	},
	{
		items:     []any{1, 2, 3, 4},
		batchSize: 0,
		expected:  [][]any{{1, 2, 3, 4}},
	},
}

func TestBatchedIterator(t *testing.T) {
	RegisterTestingT(t)

	for _, testCase := range testMatrix {
		it := iterators.Slice(toPointerSlice(testCase.items))
		batched := iterators.Batch[any](it, testCase.batchSize)
		actual, err := iterators.ReadAllPointers(batched)
		Expect(err).ToNot(HaveOccurred())
		for i := range testCase.expected {
			Expect(*actual[i]).To(Equal(toPointerSlice(testCase.expected[i])))
		}
	}
}

func toPointerSlice[V any](vs []V) []*V {
	ps := make([]*V, len(vs))
	for i, v := range vs {
		ps[i] = &v
	}
	return ps
}
