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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

func TestSecondChanceCache(t *testing.T) {
	t.Parallel()
	cache := New(2)
	require.NotNil(t, cache)

	cache.Add("a", "xyz")

	cache.Add("b", "123")
	// Get b, b referenced bit is set to true (accessed via Get). a's referenced bit is 0.
	obj, ok := cache.Get("b")
	require.True(t, ok)
	require.Equal(t, "123", obj.(string))

	obj, ok, err := cache.GetOrLoad("b", func() (any, error) { return "111", nil })
	require.True(t, ok)
	require.NoError(t, err)
	require.Equal(t, "123", obj.(string))

	obj, ok, err = cache.GetOrLoad("c", func() (any, error) { return "111", errors.New("some err") })
	require.False(t, ok)
	require.Error(t, err)
	require.Nil(t, obj)

	obj, ok, err = cache.GetOrLoad("c", func() (any, error) { return "111", nil })
	require.False(t, ok)
	require.NoError(t, err)
	require.Equal(t, "111", obj.(string))

	// check a is deleted
	_, ok = cache.Get("a")
	require.False(t, ok)

	// Add d. Adding d should trigger eviction. The second-chance algorithm should evict c
	// because c's referenced bit is 0, while b's referenced bit is 1.
	// In the process, b's referenced bit is set to 0.
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
	t.Parallel()
	cache := New(25)

	workers := 16
	wg := sync.WaitGroup{}
	wg.Add(workers)

	key1 := "key1"
	val1 := key1

	for i := range workers {
		id := i
		key2 := fmt.Sprintf("key2-%d", i)
		val2 := key2

		go func() {
			for j := range 10000 {
				key3 := fmt.Sprintf("key3-%d-%d", id, j)
				val3 := key3
				cache.Add(key3, val3)

				val, ok := cache.Get(key1)
				if ok {
					assert.Equal(t, val1, val.(string))
				}
				cache.Add(key1, val1)

				val, ok = cache.Get(key2)
				if ok {
					assert.Equal(t, val2, val.(string))
				}
				cache.Add(key2, val2)

				key4 := fmt.Sprintf("key4-%d", j)
				val4 := key4
				val, ok = cache.Get(key4)
				if ok {
					assert.Equal(t, val4, val.(string))
				}
				cache.Add(key4, val4)

				val, ok = cache.Get(key3)
				if ok {
					assert.Equal(t, val3, val.(string))
				}
			}

			wg.Done()
		}()
	}
	wg.Wait()
}

func TestSecondChanceCacheDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupFunc func() (addFunc func(any, any), getFunc func(any) (any, bool), deleteFunc func(any))
	}{
		{
			name: "TypedCache",
			setupFunc: func() (func(any, any), func(any) (any, bool), func(any)) {
				cache := New(10)
				return func(k, v any) { cache.Add(k.(string), v) },
					func(k any) (any, bool) { return cache.Get(k.(string)) },
					func(k any) { cache.Delete(k.(string)) }
			},
		},
		{
			name: "BytesCache",
			setupFunc: func() (func(any, any), func(any) (any, bool), func(any)) {
				cache := NewBytes(10)
				return func(k, v any) { cache.Add(k.([]byte), v) },
					func(k any) (any, bool) { return cache.Get(k.([]byte)) },
					func(k any) { cache.Delete(k.([]byte)) }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			add, get, del := tt.setupFunc()

			var key any
			if tt.name == "TypedCache" {
				key = "k1"
			} else {
				key = []byte("k1")
			}

			add(key, "v1")
			v, ok := get(key)
			require.True(t, ok)
			require.Equal(t, "v1", v)

			del(key)
			v, ok = get(key)
			require.True(t, ok)
			require.Nil(t, v)
		})
	}
}

func TestSecondChanceCacheBytes(t *testing.T) {
	t.Parallel()
	cache := NewBytes(2)
	k1 := []byte("key1")
	k2 := []byte("key2")
	k3 := []byte("key3")

	cache.Add(k1, "v1")
	cache.Add(k2, "v2")

	v, ok := cache.Get(k1)
	require.True(t, ok)
	require.Equal(t, "v1", v)

	// k1 is now referenced (accessed via Get). k2 is not referenced.
	// Adding k3 should trigger eviction. The second-chance algorithm should evict k2
	// because k2's referenced bit is 0, while k1's referenced bit is 1.
	cache.Add(k3, "v3")

	_, ok = cache.Get(k2)
	require.False(t, ok)

	v, ok = cache.Get(k1)
	require.True(t, ok)
	require.Equal(t, "v1", v)

	v, ok = cache.Get(k3)
	require.True(t, ok)
	require.Equal(t, "v3", v)
}

func BenchmarkSecondChanceCache(b *testing.B) {
	cache := New(b.N)
	for i := 0; i < b.N; i++ {
		// b.StopTimer()
		key := make([]byte, 64)
		_, err := rand.Read(key)
		require.NoError(b, err)
		// b.StartTimer()

		cache.Add(string(key), fmt.Sprintf("value-%d", i))
	}
}

func BenchmarkSecondChanceCacheBytes(b *testing.B) {
	cache := NewBytes(b.N)
	for i := 0; i < b.N; i++ {
		key := make([]byte, 64)
		_, err := rand.Read(key)
		require.NoError(b, err)

		cache.Add(key, fmt.Sprintf("value-%d", i))
	}
}
