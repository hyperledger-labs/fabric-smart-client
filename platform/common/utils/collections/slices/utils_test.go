/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package slices

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemove(t *testing.T) {
	t.Parallel()

	items, removed := Remove([]string{"a", "b", "c"}, "b")
	require.True(t, removed)
	require.Equal(t, []string{"a", "c"}, items)

	items, removed = Remove([]string{"a", "b"}, "z")
	require.False(t, removed)
	require.Equal(t, []string{"a", "b"}, items)

	items, removed = Remove[string](nil, "z")
	require.False(t, removed)
	require.Nil(t, items)
}

func TestDifferenceAndIntersection(t *testing.T) {
	t.Parallel()

	require.ElementsMatch(t, []string{"alice", "carol"}, Difference([]string{"alice", "bob", "carol"}, []string{"bob"}))
	require.Equal(t, []string{"carol", "alice"}, Intersection([]string{"alice", "carol"}, []string{"carol", "bob", "alice"}))
}

func TestRepeatAndSortedSliceAdd(t *testing.T) {
	t.Parallel()

	require.Equal(t, []int{7, 7, 7}, Repeat(7, 3))

	sorted := SortedSlice[int]{1, 3}
	sorted.Add(2)
	sorted.Add(3)
	require.Equal(t, SortedSlice[int]{1, 2, 3}, sorted)
}
