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

// --- Filter ---------------------------------------------------------------

func TestFilter(t *testing.T) {
	t.Parallel()

	it := iterators.Filter(iterators.Slice(ptrs(1, 2, 3, 4, 5, 6)),
		func(v *int) bool { return *v%2 == 0 })

	got, err := iterators.ReadAllValues(it)
	require.NoError(t, err)
	require.Equal(t, []int{2, 4, 6}, got)
}

func TestFilterKeepsNothing(t *testing.T) {
	t.Parallel()

	it := iterators.Filter(iterators.Slice(ptrs(1, 3, 5)),
		func(v *int) bool { return *v%2 == 0 })

	got, err := iterators.ReadAllValues(it)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestFilterKeepsEverything(t *testing.T) {
	t.Parallel()

	it := iterators.Filter(iterators.Slice(ptrs(1, 2)), func(*int) bool { return true })

	got, err := iterators.ReadAllValues(it)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, got)
}

func TestFilterEmptySource(t *testing.T) {
	t.Parallel()

	it := iterators.Filter(iterators.Slice([]*int{}), func(*int) bool { return true })

	got, err := iterators.ReadAllValues(it)
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestFilterLeadingRejectionsRecurse covers the recursive skip path: several
// consecutive rejected elements must not lose the first accepted one.
func TestFilterLeadingRejectionsRecurse(t *testing.T) {
	t.Parallel()

	it := iterators.Filter(iterators.Slice(ptrs(1, 1, 1, 1, 2)),
		func(v *int) bool { return *v == 2 })

	got, err := iterators.ReadAllValues(it)
	require.NoError(t, err)
	require.Equal(t, []int{2}, got)
}

func TestFilterPropagatesError(t *testing.T) {
	t.Parallel()

	it := iterators.Filter[int](newFailing(ptrs(1, 2), 0), func(*int) bool { return true })

	_, err := it.Next()
	require.ErrorIs(t, err, errAt)
}

// TestFilterErrorAfterRejection checks the error still surfaces when it happens
// while skipping rejected elements.
func TestFilterErrorAfterRejection(t *testing.T) {
	t.Parallel()

	it := iterators.Filter[int](newFailing(ptrs(1, 2), 1), func(v *int) bool { return *v != 1 })

	_, err := it.Next()
	require.ErrorIs(t, err, errAt)
}
