/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockEvictionPolicy struct {
	pushedKeys []string
}

func (m *mockEvictionPolicy) Push(key string) {
	m.pushedKeys = append(m.pushedKeys, key)
}

func (m *mockEvictionPolicy) String() string {
	return "mockPolicy"
}

func TestEvictionCache(t *testing.T) {
	t.Parallel()
	m := map[string]string{}
	policy := &mockEvictionPolicy{}
	c := &evictionCache[string, string]{
		m:              m,
		l:              &sync.RWMutex{},
		evictionPolicy: policy,
	}

	// Test Put and Get
	c.Put("k1", "v1")
	v, ok := c.Get("k1")
	require.True(t, ok)
	require.Equal(t, "v1", v)
	require.Contains(t, policy.pushedKeys, "k1")

	// Test Len
	require.Equal(t, 1, c.Len())

	// Test Update (existing key)
	ok = c.Update("k1", func(exists bool, val string) (bool, string) {
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
	policy.pushedKeys = nil
	ok = c.Update("k2", func(exists bool, val string) (bool, string) {
		return true, "v2"
	})
	require.False(t, ok)
	require.Contains(t, policy.pushedKeys, "k2")

	// Test Delete
	c.Delete("k2")
	require.Equal(t, 0, c.Len())

	// Test String
	require.Contains(t, c.String(), "Content")
	require.Contains(t, c.String(), "mockPolicy")
}

func TestEvictHelper(t *testing.T) {
	t.Parallel()
	m := map[string]string{
		"k1": "v1",
		"k2": "v2",
	}
	var evictedMap map[string]string
	onEvict := func(evicted map[string]string) {
		evictedMap = evicted
	}

	// Evict existing and non-existing keys
	evict([]string{"k1", "k3"}, m, onEvict)

	require.NotContains(t, m, "k1")
	require.Contains(t, m, "k2")
	require.Contains(t, evictedMap, "k1")
	require.NotContains(t, evictedMap, "k3")
	require.Equal(t, "v1", evictedMap["k1"])
}

func TestLRUString(t *testing.T) {
	t.Parallel()
	c := NewLRUCache[string, string](3, 2, nil)
	c.Put("k1", "v1")

	s := c.String()
	require.Contains(t, s, "Content")
	require.Contains(t, s, "k1")
	require.Contains(t, s, "Keys")
	require.Contains(t, s, "KeySet")
}
