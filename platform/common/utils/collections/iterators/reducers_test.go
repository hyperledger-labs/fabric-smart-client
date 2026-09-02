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

func TestNewReducerProducesInitial(t *testing.T) {
	t.Parallel()

	r := iterators.NewReducer[*int](7, func(acc int, v *int) (int, error) { return acc + *v, nil })
	require.Equal(t, 7, r.Produce())

	got, err := r.Reduce(7, new(3))
	require.NoError(t, err)
	require.Equal(t, 10, got)
}

func TestNewReducerPropagatesError(t *testing.T) {
	t.Parallel()

	mergeErr := errors.New("merge failed")
	r := iterators.NewReducer[*int](0, func(int, *int) (int, error) { return 0, mergeErr })
	_, err := r.Reduce(0, new(1))
	require.ErrorIs(t, err, mergeErr)
}

func TestToSet(t *testing.T) {
	t.Parallel()

	set, err := iterators.Reduce(iterators.Slice(ptrs(1, 2, 2, 3, 1)), iterators.ToSet[int]())
	require.NoError(t, err)
	require.Equal(t, 3, set.Length(), "duplicates collapse")
	require.True(t, set.Contains(1))
	require.True(t, set.Contains(2))
	require.True(t, set.Contains(3))
}

func TestToSetEmpty(t *testing.T) {
	t.Parallel()

	set, err := iterators.Reduce(iterators.Slice([]*int{}), iterators.ToSet[int]())
	require.NoError(t, err)
	require.Zero(t, set.Length())
}

func TestToFlattened(t *testing.T) {
	t.Parallel()

	groups := ptrs([]int{1, 2}, []int{3}, []int{4, 5})
	got, err := iterators.Reduce(iterators.Slice(groups), iterators.ToFlattened[int]())
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3, 4, 5}, got)
}

// TestToFlattenedSkipsEmptyGroups is the counterpart to the Flatten iterator's
// behaviour: the reducer handles empty groups correctly, appending nothing and
// continuing, where FlattenValues would stop.
func TestToFlattenedSkipsEmptyGroups(t *testing.T) {
	t.Parallel()

	groups := ptrs([]int{1}, []int{}, []int{2})
	got, err := iterators.Reduce(iterators.Slice(groups), iterators.ToFlattened[int]())
	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, got, "an empty group is skipped, not treated as the end")
}

func TestToFlattenedEmpty(t *testing.T) {
	t.Parallel()

	got, err := iterators.Reduce(iterators.Slice([]*[]int{}), iterators.ToFlattened[int]())
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestToMaxBy(t *testing.T) {
	t.Parallel()

	type record struct {
		name string
		size int
	}
	items := ptrs(
		record{name: "small", size: 1},
		record{name: "largest", size: 99},
		record{name: "middle", size: 50},
	)
	got, err := iterators.Reduce(
		iterators.Slice(items),
		iterators.ToMaxBy(func(r *record) (int, error) { return r.size, nil }),
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "largest", got.name)
}

// TestToMaxByTiePrefersFirst covers the strict `<` comparison: a later element
// equal to the current max does not replace it.
func TestToMaxByTiePrefersFirst(t *testing.T) {
	t.Parallel()

	type record struct {
		name string
		size int
	}
	items := ptrs(record{name: "first", size: 5}, record{name: "second", size: 5})
	got, err := iterators.Reduce(
		iterators.Slice(items),
		iterators.ToMaxBy(func(r *record) (int, error) { return r.size, nil }),
	)
	require.NoError(t, err)
	require.Equal(t, "first", got.name)
}

// TestToMaxByAllNegativeKeys covers keys below the zero value of the key type:
// the first element has to win regardless of how its key compares to that zero,
// or an all-negative iterator would reduce to nothing.
func TestToMaxByAllNegativeKeys(t *testing.T) {
	t.Parallel()

	got, err := iterators.Reduce(
		iterators.Slice(ptrs(-5, -3)),
		iterators.ToMaxBy(func(v *int) (int, error) { return *v, nil }),
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, -3, *got)
}

func TestToMaxByEmpty(t *testing.T) {
	t.Parallel()

	got, err := iterators.Reduce(
		iterators.Slice([]*int{}),
		iterators.ToMaxBy(func(v *int) (int, error) { return *v, nil }),
	)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestToMaxByTransformerError(t *testing.T) {
	t.Parallel()

	keyErr := errors.New("cannot derive key")
	_, err := iterators.Reduce(
		iterators.Slice(ptrs(1)),
		iterators.ToMaxBy(func(*int) (int, error) { return 0, keyErr }),
	)
	require.ErrorIs(t, err, keyErr)
}
