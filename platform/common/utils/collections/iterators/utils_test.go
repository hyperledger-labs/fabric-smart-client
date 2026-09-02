/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

func TestReadAllPointers(t *testing.T) {
	t.Parallel()

	items := ptrs(1, 2, 3)
	got, err := iterators.ReadAllPointers(iterators.Slice(items))
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, items, got)
}

func TestReadAllPointersEmpty(t *testing.T) {
	t.Parallel()

	got, err := iterators.ReadAllPointers(iterators.Slice([]*int{}))
	require.NoError(t, err)
	require.Empty(t, got)
	require.NotNil(t, got, "an empty read returns an allocated slice, not nil")
}

func TestReadAllPointersError(t *testing.T) {
	t.Parallel()

	it := newFailing(ptrs(1, 2, 3), 1)
	got, err := iterators.ReadAllPointers[int](it)
	require.ErrorIs(t, err, errAt)
	require.Nil(t, got)
	require.True(t, it.closed, "the iterator is closed even when reading fails")
}

func TestReadAllValues(t *testing.T) {
	t.Parallel()

	got, err := iterators.ReadAllValues(iterators.Slice(ptrs("a", "b")))
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, got)
}

func TestReadAllValuesError(t *testing.T) {
	t.Parallel()

	got, err := iterators.ReadAllValues[int](newFailing(ptrs(1, 2), 0))
	require.ErrorIs(t, err, errAt)
	require.Nil(t, got)
}

func TestReadFirst(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		items    []int
		limit    int
		expected []int
	}{
		{name: "fewer than limit", items: []int{1, 2}, limit: 5, expected: []int{1, 2}},
		{name: "exactly limit", items: []int{1, 2, 3}, limit: 3, expected: []int{1, 2, 3}},
		{name: "more than limit", items: []int{1, 2, 3, 4}, limit: 2, expected: []int{1, 2}},
		{name: "limit one", items: []int{7, 8}, limit: 1, expected: []int{7}},
		{name: "limit zero", items: []int{1, 2}, limit: 0, expected: []int{}},
		{name: "negative limit", items: []int{1, 2}, limit: -1, expected: []int{}},
		{name: "empty source", items: []int{}, limit: 3, expected: []int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := iterators.ReadFirst(iterators.Slice(ptrs(tc.items...)), tc.limit)
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestReadFirstError(t *testing.T) {
	t.Parallel()

	got, err := iterators.ReadFirst[int](newFailing(ptrs(1, 2, 3), 1), 3)
	require.ErrorIs(t, err, errAt)
	require.Nil(t, got)
}

// TestReadFirstStopsEarly pins that ReadFirst stops pulling once the limit is
// reached, rather than reading one element past it.
func TestReadFirstStopsEarly(t *testing.T) {
	t.Parallel()

	it := newInfallible(ptrs(1, 2, 3, 4))
	got, err := iterators.ReadFirst[int](it, 2)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, got)
	// An extra read would also drop any error it produced: the read that ends
	// the loop has nowhere to report one.
	require.Equal(t, 2, it.calls, "no element is read past the limit")
}

// TestReadFirstNonPositiveLimitReadsNothing pins that a limit of zero or less
// leaves the source untouched.
func TestReadFirstNonPositiveLimitReadsNothing(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{0, -1} {
		it := newInfallible(ptrs(1, 2))
		got, err := iterators.ReadFirst[int](it, limit)
		require.NoError(t, err)
		require.Empty(t, got)
		require.Zero(t, it.calls, "limit %d: the source is never read", limit)
		require.True(t, it.closed, "limit %d: the iterator is still closed", limit)
	}
}

func TestCopy(t *testing.T) {
	t.Parallel()

	original := ptrs(1, 2, 3)
	copied, err := iterators.Copy(iterators.Slice(original))
	require.NoError(t, err)

	got, err := iterators.ReadAllValues(copied)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, got)
}

// TestCopyIsIndependent checks the copy is backed by its own data rather than by
// the source: Copy drains and closes what it reads, so returning the source
// itself would hand back an exhausted iterator.
func TestCopyIsIndependent(t *testing.T) {
	t.Parallel()

	source := iterators.Slice(ptrs(1, 2))
	copied, err := iterators.Copy[int](source)
	require.NoError(t, err)

	drained, err := iterators.ReadAllValues[int](source)
	require.NoError(t, err)
	require.Empty(t, drained, "Copy consumed the source")

	got, err := iterators.ReadAllValues(copied)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, got, "the copy still yields every element")
}

func TestCopyEmpty(t *testing.T) {
	t.Parallel()

	copied, err := iterators.Copy(iterators.Slice([]*int{}))
	require.NoError(t, err)

	got, err := iterators.ReadAllValues(copied)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestCopyError(t *testing.T) {
	t.Parallel()

	copied, err := iterators.Copy[int](newFailing(ptrs(1), 0))
	require.ErrorIs(t, err, errAt)
	require.Nil(t, copied)
}

func TestGetUnique(t *testing.T) {
	t.Parallel()

	got, err := iterators.GetUnique(iterators.Slice([]int{42}))
	require.NoError(t, err)
	require.Equal(t, 42, got)
}

// TestGetUniqueDoesNotEnforceUniqueness records that GetUnique returns the first
// element without verifying there is only one, despite the name.
func TestGetUniqueDoesNotEnforceUniqueness(t *testing.T) {
	t.Parallel()

	got, err := iterators.GetUnique(iterators.Slice([]int{1, 2, 3}))
	require.NoError(t, err)
	require.Equal(t, 1, got, "GetUnique returns the head; it does not assert a single element")
}

func TestGetUniqueEmpty(t *testing.T) {
	t.Parallel()

	got, err := iterators.GetUnique(iterators.Slice([]*int{}))
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestGetUniqueError(t *testing.T) {
	t.Parallel()

	_, err := iterators.GetUnique[*int](newFailing(ptrs(1), 0))
	require.ErrorIs(t, err, errAt)
}

func TestGetFirst(t *testing.T) {
	t.Parallel()

	got, err := iterators.GetFirst(iterators.Slice([]string{"first", "second"}))
	require.NoError(t, err)
	require.Equal(t, "first", got)
}

func TestGetFirstEmpty(t *testing.T) {
	t.Parallel()

	got, err := iterators.GetFirst(iterators.Slice([]string{}))
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestGetFirstClosesIterator(t *testing.T) {
	t.Parallel()

	rec := newCloseRecorder([]int{1, 2})
	_, err := iterators.GetFirst[int](rec)
	require.NoError(t, err)
	require.True(t, rec.closed)
}

func TestForEach(t *testing.T) {
	t.Parallel()

	seen := make([]int, 0)
	err := iterators.ForEach(iterators.Slice(ptrs(1, 2, 3)), func(v *int) error {
		seen = append(seen, *v)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, seen, "elements are visited in order")
}

func TestForEachEmpty(t *testing.T) {
	t.Parallel()

	calls := 0
	err := iterators.ForEach(iterators.Slice([]*int{}), func(*int) error {
		calls++
		return nil
	})
	require.NoError(t, err)
	require.Zero(t, calls)
}

// TestForEachConsumerErrorShortCircuits asserts iteration stops at the failing
// element rather than visiting the rest.
func TestForEachConsumerErrorShortCircuits(t *testing.T) {
	t.Parallel()

	consumeErr := errors.New("consumer rejected")
	seen := make([]int, 0)
	err := iterators.ForEach(iterators.Slice(ptrs(1, 2, 3)), func(v *int) error {
		seen = append(seen, *v)
		if *v == 2 {
			return consumeErr
		}
		return nil
	})
	require.ErrorIs(t, err, consumeErr)
	require.Equal(t, []int{1, 2}, seen, "the third element is never consumed")
}

func TestForEachIteratorError(t *testing.T) {
	t.Parallel()

	it := newFailing(ptrs(1, 2), 1)
	calls := 0
	err := iterators.ForEach[int](it, func(*int) error {
		calls++
		return nil
	})
	require.ErrorIs(t, err, errAt)
	require.Equal(t, 1, calls)
	require.True(t, it.closed)
}

func TestReduceValue(t *testing.T) {
	t.Parallel()

	sum, err := iterators.ReduceValue(iterators.Slice(ptrs(1, 2, 3, 4)), 0,
		func(acc int, v *int) (int, error) { return acc + *v, nil })
	require.NoError(t, err)
	require.Equal(t, 10, sum)
}

func TestReduceValueEmptyReturnsInitial(t *testing.T) {
	t.Parallel()

	sum, err := iterators.ReduceValue(iterators.Slice([]*int{}), 99,
		func(acc int, v *int) (int, error) { return acc + *v, nil })
	require.NoError(t, err)
	require.Equal(t, 99, sum, "an empty iterator yields the initial value untouched")
}

// TestReduceValueReduceErrorReturnsZero pins the contract that a reduce failure
// discards the accumulator rather than returning it partially built.
func TestReduceValueReduceErrorReturnsZero(t *testing.T) {
	t.Parallel()

	reduceErr := errors.New("cannot reduce")
	sum, err := iterators.ReduceValue(iterators.Slice(ptrs(1, 2, 3)), 100,
		func(acc int, v *int) (int, error) {
			if *v == 2 {
				return 0, reduceErr
			}
			return acc + *v, nil
		})
	require.ErrorIs(t, err, reduceErr)
	require.Zero(t, sum, "the partially accumulated value is discarded on error")
}

func TestReduceValueIteratorError(t *testing.T) {
	t.Parallel()

	it := newFailing(ptrs(1, 2), 1)
	sum, err := iterators.ReduceValue[int, int](it, 5,
		func(acc int, v *int) (int, error) { return acc + *v, nil })
	require.ErrorIs(t, err, errAt)
	require.Zero(t, sum)
	require.True(t, it.closed)
}

func TestReduce(t *testing.T) {
	t.Parallel()

	reducer := iterators.NewReducer[*int](0, func(acc int, v *int) (int, error) { return acc + *v, nil })
	sum, err := iterators.Reduce(iterators.Slice(ptrs(2, 3, 5)), reducer)
	require.NoError(t, err)
	require.Equal(t, 10, sum)
}

func TestReduceError(t *testing.T) {
	t.Parallel()

	reduceErr := errors.New("reducer failed")
	reducer := iterators.NewReducer[*int](0, func(int, *int) (int, error) { return 0, reduceErr })
	_, err := iterators.Reduce(iterators.Slice(ptrs(1)), reducer)
	require.ErrorIs(t, err, reduceErr)
}
