/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package maps

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCopyOverwritesExistingKeys(t *testing.T) {
	t.Parallel()

	target := map[string]int{"existing": 1}
	Copy(target, map[string]int{"existing": 2, "new": 3})
	require.Equal(t, map[string]int{"existing": 2, "new": 3}, target)
}

func TestCopyIgnoresNilSource(t *testing.T) {
	t.Parallel()

	target := map[string]int{"existing": 1}
	Copy(target, nil)
	require.Equal(t, map[string]int{"existing": 1}, target)
}

func TestInverse(t *testing.T) {
	t.Parallel()

	input := map[string]int{"alice": 1, "bob": 2}

	require.Equal(t, map[int]string{1: "alice", 2: "bob"}, Inverse(input))
}

func TestContainsValue(t *testing.T) {
	t.Parallel()

	input := map[string]int{"alice": 1, "bob": 2}

	require.True(t, ContainsValue(input, 2))
	require.False(t, ContainsValue(input, 3))
}

func TestKeys(t *testing.T) {
	t.Parallel()

	input := map[string]int{"alice": 1, "bob": 2}

	require.ElementsMatch(t, []string{"alice", "bob"}, Keys(input))
}

func TestValues(t *testing.T) {
	t.Parallel()

	input := map[string]int{"alice": 1, "bob": 2}

	require.ElementsMatch(t, []int{1, 2}, Values(input))
}

func TestSubMap(t *testing.T) {
	t.Parallel()

	found, missing := SubMap(map[string]int{"alice": 1, "bob": 2}, "bob", "carol")

	require.Equal(t, map[string]int{"bob": 2}, found)
	require.Equal(t, []string{"carol"}, missing)
}

func TestRepeatValue(t *testing.T) {
	t.Parallel()

	require.Equal(t, map[string]bool{"alice": true, "bob": true}, RepeatValue([]string{"alice", "bob"}, true))
}
