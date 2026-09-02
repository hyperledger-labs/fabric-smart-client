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

// flattenedPointers and flattenedValues are near-duplicate implementations that
// can drift apart, so the cases that apply to both are declared once here and run
// against each variant.
//
// The two differ only in what they yield: Flatten passes the element type through
// (so a []*int group yields *int), while FlattenValues yields a pointer to each
// element (a []int group yields *int). Both are driven below over groups of int,
// which is the shape that lets one table cover them.
type flattenVariant struct {
	name string
	// flattenInts builds an iterator over the given groups, yielding one *int per
	// element, whatever the underlying representation.
	flattenInts func(groups [][]int) iterators.Iterator[*int]
	// flattenFailing does the same over a source that fails at failAt.
	flattenFailing func(groups [][]int, failAt int) iterators.Iterator[*int]
	// flattenBadTransformer returns an iterator whose transformer always fails.
	flattenBadTransformer func(groups [][]int, err error) iterators.Iterator[*int]
}

var flattenVariants = []flattenVariant{
	{
		name: "FlattenValues",
		flattenInts: func(groups [][]int) iterators.Iterator[*int] {
			return iterators.FlattenValues(iterators.Slice(groups),
				func(g []int) ([]int, error) { return g, nil })
		},
		flattenFailing: func(groups [][]int, failAt int) iterators.Iterator[*int] {
			return iterators.FlattenValues(newFailing(groups, failAt),
				func(g []int) ([]int, error) { return g, nil })
		},
		flattenBadTransformer: func(groups [][]int, err error) iterators.Iterator[*int] {
			return iterators.FlattenValues(iterators.Slice(groups),
				func([]int) ([]int, error) { return nil, err })
		},
	},
	{
		name: "Flatten",
		flattenInts: func(groups [][]int) iterators.Iterator[*int] {
			return iterators.Flatten(iterators.Slice(toPointerGroups(groups)),
				func(g []*int) ([]*int, error) { return g, nil })
		},
		flattenFailing: func(groups [][]int, failAt int) iterators.Iterator[*int] {
			return iterators.Flatten(newFailing(toPointerGroups(groups), failAt),
				func(g []*int) ([]*int, error) { return g, nil })
		},
		flattenBadTransformer: func(groups [][]int, err error) iterators.Iterator[*int] {
			return iterators.Flatten(iterators.Slice(toPointerGroups(groups)),
				func([]*int) ([]*int, error) { return nil, err })
		},
	},
}

func toPointerGroups(groups [][]int) [][]*int {
	out := make([][]*int, 0, len(groups))
	for _, g := range groups {
		out = append(out, ptrs(g...))
	}
	return out
}

// drainPointers reads a flattened iterator to exhaustion. Exhaustion is signalled
// by a nil item, matching what ReadAllPointers expects.
func drainPointers(tb testing.TB, it iterators.Iterator[*int]) []int {
	tb.Helper()
	out := make([]int, 0)
	for {
		item, err := it.Next()
		require.NoError(tb, err)
		if item == nil {
			return out
		}
		out = append(out, *item)
	}
}

func TestFlattenBothVariants(t *testing.T) {
	t.Parallel()

	for _, v := range flattenVariants {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()

			t.Run("several groups", func(t *testing.T) {
				t.Parallel()
				it := v.flattenInts([][]int{{1, 2}, {3}, {4, 5, 6}})
				require.Equal(t, []int{1, 2, 3, 4, 5, 6}, drainPointers(t, it))
			})

			t.Run("single group", func(t *testing.T) {
				t.Parallel()
				require.Equal(t, []int{42}, drainPointers(t, v.flattenInts([][]int{{42}})))
			})

			t.Run("single element groups", func(t *testing.T) {
				t.Parallel()
				it := v.flattenInts([][]int{{1}, {2}, {3}})
				require.Equal(t, []int{1, 2, 3}, drainPointers(t, it))
			})

			t.Run("empty outer", func(t *testing.T) {
				t.Parallel()
				require.Empty(t, drainPointers(t, v.flattenInts([][]int{})))
			})

			// Pins the restriction documented on Flatten and FlattenValues: an empty
			// group is indistinguishable from exhaustion, so the trailing group is
			// never observed. If Next() is ever changed to loop on the empty case,
			// invert this to expect {1, 2, 3} and update the godoc with it.
			t.Run("stops at empty inner group", func(t *testing.T) {
				t.Parallel()
				it := v.flattenInts([][]int{{1, 2}, {}, {3}})
				require.Equal(t, []int{1, 2}, drainPointers(t, it),
					"an empty inner group ends iteration")
			})

			t.Run("leading empty group yields nothing", func(t *testing.T) {
				t.Parallel()
				require.Empty(t, drainPointers(t, v.flattenInts([][]int{{}, {1}})))
			})

			t.Run("underlying error", func(t *testing.T) {
				t.Parallel()
				_, err := v.flattenFailing([][]int{{1}}, 0).Next()
				require.ErrorIs(t, err, errAt)
				require.ErrorContains(t, err, "failed fetching")
			})

			t.Run("transformer error", func(t *testing.T) {
				t.Parallel()
				transformErr := errors.New("cannot transform")
				_, err := v.flattenBadTransformer([][]int{{1}}, transformErr).Next()
				require.ErrorIs(t, err, transformErr)
				require.ErrorContains(t, err, "failed transforming")
			})

			// Next() past the end must stay stable rather than panic or report an
			// index error.
			t.Run("next after exhaustion is stable", func(t *testing.T) {
				t.Parallel()
				it := v.flattenInts([][]int{{1}})
				require.Equal(t, []int{1}, drainPointers(t, it))
				for range 3 {
					item, err := it.Next()
					require.NoError(t, err)
					require.Nil(t, item)
				}
			})
		})
	}
}

// TestFlattenValuesTransformerErrorMidStream checks the error surfaces only once
// the buffered elements of the previous group are drained, rather than as soon as
// the failing group is reached.
func TestFlattenValuesTransformerErrorMidStream(t *testing.T) {
	t.Parallel()

	transformErr := errors.New("boom on second group")
	calls := 0
	it := iterators.FlattenValues(
		iterators.Slice([][]int{{1, 2}, {3}}),
		func(g []int) ([]int, error) {
			calls++
			if calls == 2 {
				return nil, transformErr
			}
			return g, nil
		},
	)

	first, err := it.Next()
	require.NoError(t, err)
	require.Equal(t, 1, *first)

	second, err := it.Next()
	require.NoError(t, err)
	require.Equal(t, 2, *second)

	_, err = it.Next()
	require.ErrorIs(t, err, transformErr)
}

// TestFlattenValuesClosePropagates covers the embedded Iterator's Close, which is
// shared by both variants.
func TestFlattenValuesClosePropagates(t *testing.T) {
	t.Parallel()

	rec := newCloseRecorder([][]int{{1}})
	it := iterators.FlattenValues(rec, func(g []int) ([]int, error) { return g, nil })
	it.Close()
	require.True(t, rec.closed, "Close must reach the wrapped iterator")
}

// TestFlattenValuesYieldsDistinctPointers guards against the loop-variable
// aliasing bug that would make every yielded pointer refer to the same element.
func TestFlattenValuesYieldsDistinctPointers(t *testing.T) {
	t.Parallel()

	it := iterators.FlattenValues(
		iterators.Slice([][]int{{1, 2, 3}}),
		func(g []int) ([]int, error) { return g, nil },
	)

	first, err := it.Next()
	require.NoError(t, err)
	second, err := it.Next()
	require.NoError(t, err)

	require.NotSame(t, first, second)
	require.Equal(t, 1, *first, "the first pointer still refers to the first element")
	require.Equal(t, 2, *second)
}
