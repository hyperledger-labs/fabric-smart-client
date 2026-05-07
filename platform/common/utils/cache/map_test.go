/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapCache(t *testing.T) {
	t.Parallel()
	c := NewMapCache[string, string]()

	// Test Put and Get
	c.Put("k1", "v1")
	v, ok := c.Get("k1")
	require.True(t, ok)
	require.Equal(t, "v1", v)

	// Test Len
	require.Equal(t, 1, c.Len())

	// Test Update (existing key)
	ok = c.Update("k1", func(exists bool, val string) (bool, string) {
		require.True(t, exists)
		require.Equal(t, "v1", val)
		return true, "v1-updated"
	})
	require.True(t, ok)
	v, ok = c.Get("k1")
	require.True(t, ok)
	require.Equal(t, "v1-updated", v)

	// Test Update (delete key)
	c.Update("k1", func(exists bool, val string) (bool, string) {
		return false, ""
	})
	_, ok = c.Get("k1")
	require.False(t, ok)
	require.Equal(t, 0, c.Len())

	// Test Update (new key)
	ok = c.Update("k2", func(exists bool, val string) (bool, string) {
		require.False(t, exists)
		return true, "v2"
	})
	require.False(t, ok)
	v, ok = c.Get("k2")
	require.True(t, ok)
	require.Equal(t, "v2", v)

	// Test Delete
	c.Put("k3", "v3")
	require.Equal(t, 2, c.Len())
	c.Delete("k2", "k3")
	require.Equal(t, 0, c.Len())
}
