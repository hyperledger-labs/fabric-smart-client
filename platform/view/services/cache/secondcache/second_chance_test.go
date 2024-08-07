/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package secondcache

import (
	"crypto/rand"
	"fmt"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecondChanceCache(t *testing.T) {
	cache := New(2)
	require.NotNil(t, cache)

	cache.Add("a", "xyz")

	cache.Add("b", "123")
	// Get b, b referenced bit is set to true
	obj, ok := cache.Get("b")
	require.True(t, ok)
	require.Equal(t, "123", obj.(string))

	obj, ok, err := cache.GetOrLoad("b", func() (interface{}, error) { return "111", nil })
	require.True(t, ok)
	require.NoError(t, err)
	require.Equal(t, "123", obj.(string))

	obj, ok, err = cache.GetOrLoad("c", func() (interface{}, error) { return "111", errors.New("some err") })
	require.False(t, ok)
	require.Error(t, err)
	require.Equal(t, nil, obj)

	obj, ok, err = cache.GetOrLoad("c", func() (interface{}, error) { return "111", nil })
	require.False(t, ok)
	require.NoError(t, err)
	require.Equal(t, "111", obj.(string))

	// check a is deleted
	_, ok = cache.Get("a")
	require.False(t, ok)

	// Add d. victim scan: b referenced bit is set to false and delete c
	cache.Add("d", "555")

	// check c is deleted
	_, ok = cache.Get("c")
	require.False(t, ok)

	// check b and d
	obj, ok = cache.Get("b")
	require.True(t, ok)
	require.Equal(t, "123", obj.(string))
	obj, ok = cache.Get("d")
	require.True(t, ok)
	require.Equal(t, "555", obj.(string))
}

func TestSecondChanceCacheConcurrent(t *testing.T) {
	cache := New(25)

	workers := 16
	wg := sync.WaitGroup{}
	wg.Add(workers)

	key1 := "key1"
	val1 := key1

	for i := 0; i < workers; i++ {
		id := i
		key2 := fmt.Sprintf("key2-%d", i)
		val2 := key2

		go func() {
			for j := 0; j < 10000; j++ {
				key3 := fmt.Sprintf("key3-%d-%d", id, j)
				val3 := key3
				cache.Add(key3, val3)

				val, ok := cache.Get(key1)
				if ok {
					require.Equal(t, val1, val.(string))
				}
				cache.Add(key1, val1)

				val, ok = cache.Get(key2)
				if ok {
					require.Equal(t, val2, val.(string))
				}
				cache.Add(key2, val2)

				key4 := fmt.Sprintf("key4-%d", j)
				val4 := key4
				val, ok = cache.Get(key4)
				if ok {
					require.Equal(t, val4, val.(string))
				}
				cache.Add(key4, val4)

				val, ok = cache.Get(key3)
				if ok {
					require.Equal(t, val3, val.(string))
				}
			}

			wg.Done()
		}()
	}
	wg.Wait()
}

func BenchmarkSecondChanceCache(b *testing.B) {
	cache := New(b.N)
	for i := 0; i < b.N; i++ {
		// b.StopTimer()
		key := make([]byte, 64)
		_, err := rand.Read(key)
		assert.NoError(b, err)
		// b.StartTimer()

		cache.Add(string(key), fmt.Sprintf("value-%d", i))
	}
}

func BenchmarkSecondChanceCacheBytes(b *testing.B) {
	cache := NewBytes(b.N)
	for i := 0; i < b.N; i++ {
		key := make([]byte, 64)
		_, err := rand.Read(key)
		assert.NoError(b, err)

		cache.Add(key, fmt.Sprintf("value-%d", i))
	}
}
