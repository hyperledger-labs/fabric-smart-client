/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

type entry struct{ key, value string }

func TestNoErrors(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	// Get non existing
	val, err := cache.Get(entry{"key", "v1"})
	require.NoError(t, err)
	require.Equal(t, "v1", val)

	// Get existing
	val, err = cache.Get(entry{"key", "v2"})
	require.NoError(t, err)
	require.Equal(t, "v1", val)

	// Update existing
	oldVal, newVal, err := cache.Update(entry{"key", "v3"})
	require.NoError(t, err)
	require.Equal(t, "v1", oldVal)
	require.Equal(t, "v3", newVal)

	// Get updated
	val, err = cache.Get(entry{"key", "v4"})
	require.NoError(t, err)
	require.Equal(t, "v3", val)

	// Delete existing
	val, ok := cache.Delete(entry{"key", "v5"})
	require.True(t, ok)
	require.Equal(t, "v3", val)

	// Get deleted
	val, err = cache.Get(entry{"key", "v6"})
	require.NoError(t, err)
	require.Equal(t, "v6", val)
}

func TestDeleteNonExisting(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	val, ok := cache.Delete(entry{"key", "v1"})
	require.False(t, ok)
	require.Empty(t, val)
}

func TestUpdateNonExisting(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	oldVal, newVal, err := cache.Update(entry{"key", "v1"})
	require.NoError(t, err)
	require.Empty(t, oldVal)
	require.Equal(t, "v1", newVal)
}

func TestError(t *testing.T) {
	t.Parallel()
	cache := newTestCache()

	val, err := cache.Get(entry{"error", "e1"})
	require.Error(t, err, "e1")
	require.Empty(t, val)

	val, err = cache.Get(entry{"key", "v1"})
	require.NoError(t, err)
	require.Equal(t, "v1", val)
}

func TestProviderMethods(t *testing.T) {
	t.Parallel()

	// Test NewProvider
	p := NewProvider(func(k string) (string, error) {
		return k + "-val", nil
	})

	val, err := p.Get("k1")
	require.NoError(t, err)
	require.Equal(t, "k1-val", val)

	// Test Peek
	v, ok := p.Peek("k1")
	require.True(t, ok)
	require.Equal(t, "k1-val", v)

	v, ok = p.Peek("k2")
	require.False(t, ok)
	require.Empty(t, v)

	// Test Length
	require.Equal(t, 1, p.Length())
}

func TestProviderErrors(t *testing.T) {
	t.Parallel()

	p := NewProvider(func(k string) (string, error) {
		if k == "error" {
			return "", errors.New("provider error")
		}
		return k, nil
	})

	// Test Get error
	val, err := p.Get("error")
	require.Error(t, err)
	require.Equal(t, "provider error", err.Error())
	require.Empty(t, val)

	// Test Update error
	oldVal, newVal, err := p.Update("error")
	require.Error(t, err)
	require.Equal(t, "provider error", err.Error())
	require.Empty(t, oldVal)
	require.Empty(t, newVal)
}

func TestParallel(t *testing.T) {
	t.Parallel()
	const iterations = 100
	cache := newTestCache()
	vals := make(chan string, iterations)
	done := make(chan struct{})

	values := make(map[string]struct{})
	go func() {
		for v := range vals {
			values[v] = struct{}{}
		}
		done <- struct{}{}
	}()

	var wg sync.WaitGroup
	wg.Add(iterations)
	for i := 0; i < iterations; i++ {
		val := fmt.Sprintf("v%d", i)
		go func() {
			val, err := cache.Get(entry{"key", val})
			assert.NoError(t, err)
			vals <- val
			wg.Done()
		}()
	}
	wg.Wait()
	close(vals)

	<-done
	require.Equal(t, 1, cache.Length(), "we only updated one key")
	require.Len(t, values, 1, "we always got one value back (the one we first set)")
}

// TestLengthConcurrentWithWrites exercises Length alongside Update / Delete
// to expose the missing read lock. Without the RLock fix on Length, this
// test panics under `go test -race` with "concurrent map iteration and map
// write" or trips the race detector.
func TestLengthConcurrentWithWrites(t *testing.T) {
	t.Parallel()
	const iterations = 200
	cache := newTestCache()

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_, _, _ = cache.Update(entry{key: fmt.Sprintf("k%d", i), value: fmt.Sprintf("v%d", i)})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_, _ = cache.Delete(entry{key: fmt.Sprintf("k%d", i)})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = cache.Length()
		}
	}()

	wg.Wait()
}

func newTestCache() Provider[entry, string] {
	return NewProviderWithKeyMapper(func(in entry) string {
		return in.key
	}, func(in entry) (string, error) {
		if in.key == "error" {
			return "", errors.New(in.value)
		}
		return in.value, nil
	})
}
