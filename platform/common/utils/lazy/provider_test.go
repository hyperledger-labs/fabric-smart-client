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
