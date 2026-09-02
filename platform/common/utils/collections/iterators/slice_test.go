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

// --- Slice / From / Permutate --------------------------------------------

func TestSlice(t *testing.T) {
	t.Parallel()

	it := iterators.Slice([]int{10, 20})

	first, err := it.Next()
	require.NoError(t, err)
	require.Equal(t, 10, first)

	second, err := it.Next()
	require.NoError(t, err)
	require.Equal(t, 20, second)

	past, err := it.Next()
	require.NoError(t, err)
	require.Zero(t, past, "past the end yields the zero value")
}

func TestSliceEmpty(t *testing.T) {
	t.Parallel()

	v, err := iterators.Slice([]int{}).Next()
	require.NoError(t, err)
	require.Zero(t, v)
}

func TestSliceCloseReleasesItems(t *testing.T) {
	t.Parallel()

	it := iterators.Slice([]int{1, 2})
	it.Close()

	v, err := it.Next()
	require.NoError(t, err)
	require.Zero(t, v, "a closed slice iterator yields nothing further")
}

func TestFrom(t *testing.T) {
	t.Parallel()

	got, err := iterators.ReadAllValues(iterators.From(ptrs(1, 2, 3)...))
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, got)
}

func TestFromNoArgs(t *testing.T) {
	t.Parallel()

	got, err := iterators.ReadAllValues(iterators.From[*int]())
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestPermutatePreservesElements(t *testing.T) {
	t.Parallel()

	it, err := iterators.Permutate(iterators.Slice(ptrs(1, 2, 3, 4, 5)))
	require.NoError(t, err)

	got, err := iterators.ReadAllValues(it)
	require.NoError(t, err)
	require.ElementsMatch(t, []int{1, 2, 3, 4, 5}, got, "a permutation keeps the same multiset")
}

func TestPermutateEmpty(t *testing.T) {
	t.Parallel()

	it, err := iterators.Permutate(iterators.Slice([]*int{}))
	require.NoError(t, err)

	got, err := iterators.ReadAllValues(it)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestPermutateError(t *testing.T) {
	t.Parallel()

	it, err := iterators.Permutate[int](newFailing(ptrs(1), 0))
	require.ErrorIs(t, err, errAt)
	require.Nil(t, it)
}

func TestNewPermutationPreservesElements(t *testing.T) {
	t.Parallel()

	base := iterators.Slice(ptrs(1, 2, 3, 4))
	perm := base.NewPermutation()

	got, err := iterators.ReadAllValues(perm)
	require.NoError(t, err)
	require.ElementsMatch(t, []int{1, 2, 3, 4}, got)
}

func TestNewPermutationEmpty(t *testing.T) {
	t.Parallel()

	perm := iterators.Slice([]*int{}).NewPermutation()

	got, err := iterators.ReadAllValues(perm)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestNewPermutationCloseReleasesItems(t *testing.T) {
	t.Parallel()

	perm := iterators.Slice([]int{1, 2}).NewPermutation()
	perm.Close()

	v, err := perm.Next()
	require.NoError(t, err)
	require.Zero(t, v)
}
