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

// --- Predicates -----------------------------------------------------------

func TestDuplicatesBy(t *testing.T) {
	t.Parallel()

	type user struct {
		id   int
		name string
	}
	items := ptrs(
		user{id: 1, name: "a"},
		user{id: 2, name: "b"},
		user{id: 1, name: "a-again"},
		user{id: 3, name: "c"},
		user{id: 2, name: "b-again"},
	)

	it := iterators.Filter(iterators.Slice(items), iterators.DuplicatesBy(func(u *user) int { return u.id }))
	got, err := iterators.ReadAllValues(it)
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, []string{"a", "b", "c"}, []string{got[0].name, got[1].name, got[2].name},
		"the first occurrence of each key is kept")
}

func TestDuplicatesByAllUnique(t *testing.T) {
	t.Parallel()

	pred := iterators.DuplicatesBy(func(v *int) int { return *v })
	items := ptrs(1, 2, 3)
	for _, item := range items {
		require.True(t, pred(item))
	}
}

func TestDuplicatesByIsStateful(t *testing.T) {
	t.Parallel()

	pred := iterators.DuplicatesBy(func(v *int) int { return *v })
	v := new(42)
	require.True(t, pred(v), "first sighting is kept")
	require.False(t, pred(v), "the same key is rejected afterwards")
}

func TestOr(t *testing.T) {
	t.Parallel()

	isEven := func(v *int) bool { return *v%2 == 0 }
	isBig := func(v *int) bool { return *v > 100 }
	pred := iterators.Or(isEven, isBig)

	for _, tc := range []struct {
		name     string
		value    int
		expected bool
	}{
		{name: "left true", value: 4, expected: true},
		{name: "right true", value: 101, expected: true},
		{name: "both true", value: 200, expected: true},
		{name: "neither", value: 3, expected: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, pred(new(tc.value)))
		})
	}
}

// TestOrShortCircuits checks the right-hand predicate is not consulted once the
// left one accepts.
func TestOrShortCircuits(t *testing.T) {
	t.Parallel()

	rightCalls := 0
	pred := iterators.Or(
		func(*int) bool { return true },
		func(*int) bool { rightCalls++; return true },
	)
	require.True(t, pred(new(1)))
	require.Zero(t, rightCalls)
}
