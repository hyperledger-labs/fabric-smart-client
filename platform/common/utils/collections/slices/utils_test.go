/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package slices

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDifference(t *testing.T) {
	t.Parallel()

	require.ElementsMatch(t, []string{"alice", "carol"}, Difference([]string{"alice", "bob", "carol"}, []string{"bob"}))
}

func TestIntersection(t *testing.T) {
	t.Parallel()

	require.ElementsMatch(t, []string{"alice", "carol"}, Intersection([]string{"alice", "carol"}, []string{"carol", "bob", "alice"}))
}

func TestRepeat(t *testing.T) {
	t.Parallel()

	require.Equal(t, []int{7, 7, 7}, Repeat(7, 3))
}

func TestSortedSliceAdd(t *testing.T) {
	t.Parallel()

	sorted := SortedSlice[int]{1, 3}
	sorted.Add(2)
	sorted.Add(3)

	require.Equal(t, SortedSlice[int]{1, 2, 3}, sorted)
}
