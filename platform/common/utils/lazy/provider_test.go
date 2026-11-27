/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import (
	"fmt"
	"sync"
	"testing"
	"time"

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
	for i := range iterations {
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
		for i := range iterations {
			_, _, _ = cache.Update(entry{key: fmt.Sprintf("k%d", i), value: fmt.Sprintf("v%d", i)})
		}
	}()

	go func() {
		defer wg.Done()
		for i := range iterations {
			_, _ = cache.Delete(entry{key: fmt.Sprintf("k%d", i)})
		}
	}()

	go func() {
		defer wg.Done()
		for range iterations {
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

// TestSlowBuildDoesNotBlockOtherKeys holds the build of one key open and
// checks that every operation on other keys still completes.
func TestSlowBuildDoesNotBlockOtherKeys(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	p := NewProvider(func(k string) (string, error) {
		if k == "slow" {
			close(started)
			<-release
		}
		return k, nil
	})

	_, err := p.Get("cached")
	require.NoError(t, err)

	go func() { _, _ = p.Get("slow") }()
	<-started

	done := make(chan struct{})
	go func() {
		defer close(done)
		v, err := p.Get("cached")
		assert.NoError(t, err)
		assert.Equal(t, "cached", v)
		v, err = p.Get("other")
		assert.NoError(t, err)
		assert.Equal(t, "other", v)
		_, _, err = p.Update("other")
		assert.NoError(t, err)
		_, ok := p.Peek("cached")
		assert.True(t, ok)
		_, _ = p.Delete("other")
		_ = p.Length()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("operations on other keys blocked behind the build of key slow")
	}
	// let the goroutine finish before the test ends, so a late assert does not panic
	close(release)
	<-done
}

// TestDeleteWaitsForBuildOfSameKey checks that a Delete issued while its key
// is being built evicts the value of that build instead of missing it.
func TestDeleteWaitsForBuildOfSameKey(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	p := NewProvider(func(k string) (string, error) {
		close(started)
		<-release
		return k, nil
	})

	built := make(chan struct{})
	go func() {
		defer close(built)
		_, _ = p.Get("k")
	}()
	<-started

	deleted := make(chan bool)
	go func() {
		_, ok := p.Delete("k")
		deleted <- ok
	}()

	select {
	case <-deleted:
		t.Fatal("Delete returned while the build of its key was running")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-built

	require.True(t, <-deleted, "Delete evicts the value of the build it waited for")
	require.Zero(t, p.Length())
	require.Empty(t, p.keyLocks, "key locks are released once no call holds them")
}

// TestSameKeyCallsAreSerialized runs Get, Update and Delete of one key
// concurrently and checks that no two builds of that key overlap.
func TestSameKeyCallsAreSerialized(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	running, maxRunning := 0, 0
	p := NewProvider(func(k string) (string, error) {
		mu.Lock()
		running++
		maxRunning = max(maxRunning, running)
		mu.Unlock()
		time.Sleep(time.Millisecond)
		mu.Lock()
		running--
		mu.Unlock()
		return k, nil
	})

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { _, _ = p.Get("k") })
		wg.Go(func() { _, _, _ = p.Update("k") })
		wg.Go(func() { _, _ = p.Delete("k") })
	}
	wg.Wait()

	require.Equal(t, 1, maxRunning, "builds of one key never overlap")
	require.Empty(t, p.keyLocks, "key locks are released once no call holds them")
}
