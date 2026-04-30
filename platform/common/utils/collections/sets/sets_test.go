/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewContainsAndLength(t *testing.T) {
	t.Parallel()

	s := New("alice", "bob")

	require.True(t, s.Contains("alice"))
	require.Equal(t, 2, s.Length())
}

func TestEmpty(t *testing.T) {
	t.Parallel()

	require.True(t, New[string]().Empty())
	require.False(t, New("alice").Empty())
}

func TestAdd(t *testing.T) {
	t.Parallel()

	s := New("alice", "bob")
	s.Add("carol", "alice")

	require.True(t, s.Contains("carol"))
	require.Equal(t, 3, s.Length())
}

func TestRemove(t *testing.T) {
	t.Parallel()

	s := New("alice", "bob", "carol")
	s.Remove("bob")

	require.False(t, s.Contains("bob"))
	require.ElementsMatch(t, []string{"alice", "carol"}, s.ToSlice())
}
