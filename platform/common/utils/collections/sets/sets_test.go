/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetOperations(t *testing.T) {
	t.Parallel()

	s := New("alice", "bob")
	require.True(t, s.Contains("alice"))
	require.False(t, s.Empty())
	require.Equal(t, 2, s.Length())

	s.Add("carol", "alice")
	require.True(t, s.Contains("carol"))
	require.Equal(t, 3, s.Length())

	s.Remove("bob")
	require.False(t, s.Contains("bob"))
	require.ElementsMatch(t, []string{"alice", "carol"}, s.ToSlice())
}

func TestMinus(t *testing.T) {
	t.Parallel()

	diff := New("alice", "bob", "carol").Minus(New("bob"))
	require.ElementsMatch(t, []string{"alice", "carol"}, diff.ToSlice())
}
