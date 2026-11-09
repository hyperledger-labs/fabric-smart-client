/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestIdentityCache(t *testing.T) {
	t.Parallel()

	var counter atomic.Int32
	c := NewIdentityCache(func(_ *driver.IdentityOptions) (view.Identity, []byte, error) {
		counter.Add(1)
		return []byte("hello world"), []byte("audit"), nil
	}, 100, nil, nil)
	defer c.Close()

	// fetch from backend directly +1
	id, audit, err := c.Identity(&driver.IdentityOptions{
		EIDExtension: true,
		AuditInfo:    nil,
	})
	require.NoError(t, err)
	require.Equal(t, view.Identity([]byte("hello world")), id)
	require.Equal(t, []byte("audit"), audit)

	// fetch from cache (100 generated + 1 that cannot enter the queue) + 2 for the subsequent calls
	id, audit, err = c.Identity(nil)
	require.NoError(t, err)
	require.Equal(t, view.Identity([]byte("hello world")), id)
	require.Equal(t, []byte("audit"), audit)

	// fetch from cache
	id, audit, err = c.Identity(nil)
	require.NoError(t, err)
	require.Equal(t, view.Identity([]byte("hello world")), id)
	require.Equal(t, []byte("audit"), audit)

	require.Eventually(t, func() bool {
		return counter.Load() >= int32(104)
	}, 5*time.Second, 10*time.Millisecond, "expected at least 104 identities to be generated")
}

// Close stops provisioning: identities already cached are still served, nothing new is
// cached or fetched in the background, and once the cache is empty Identity falls back to
// the backend. The test runs in a synctest bubble, so synctest.Wait settles the provisioning
// goroutine instead of polling for it, and the cache timeout passes on the bubble's clock.
func TestIdentityCacheClose(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var counter atomic.Int32
		c := NewIdentityCache(func(_ *driver.IdentityOptions) (view.Identity, []byte, error) {
			counter.Add(1)
			return []byte("hello world"), []byte("audit"), nil
		}, 10, nil, nil)

		// Trigger cache initialization
		id, audit, err := c.Identity(nil)
		require.NoError(t, err)
		require.Equal(t, view.Identity([]byte("hello world")), id)
		require.Equal(t, []byte("audit"), audit)

		// The provisioning goroutine fills the cache and blocks on the full channel.
		synctest.Wait()
		require.Equal(t, 10, c.CacheSize(), "cache should fill up to capacity")

		// Close returns once the goroutine has stopped, so the size and the backend count
		// read after it no longer change.
		c.Close()
		initialCacheSize := c.CacheSize()
		initialCounter := counter.Load()

		// Exhaust all cached identities
		for range initialCacheSize {
			id, audit, err = c.Identity(nil)
			require.NoError(t, err)
			require.Equal(t, view.Identity([]byte("hello world")), id)
			require.Equal(t, []byte("audit"), audit)
		}
		require.Equal(t, 0, c.CacheSize(), "cache should be empty after exhausting all identities")

		// A provisioning goroutine still running would refill the cache before Wait returns.
		synctest.Wait()
		require.Equal(t, 0, c.CacheSize(), "cache should remain empty after close")
		require.Equal(t, initialCounter, counter.Load(), "no new identities should be generated after close")

		// With the cache empty, Identity waits out the cache timeout and asks the backend.
		id, audit, err = c.Identity(nil)
		require.NoError(t, err)
		require.Equal(t, view.Identity([]byte("hello world")), id)
		require.Equal(t, []byte("audit"), audit)
		require.Equal(t, initialCounter+1, counter.Load(), "identity should come from backend after cache exhausted")
		require.Equal(t, 0, c.CacheSize(), "cache should remain empty after fetching from backend")
	})
}

// A zero-sized cache has nothing provisioning it, so Identity must go straight to the
// backend instead of waiting out the cache timeout on every call.
func TestIdentityCacheDisabled(t *testing.T) {
	t.Parallel()
	var counter atomic.Int32
	c := NewIdentityCache(func(_ *driver.IdentityOptions) (view.Identity, []byte, error) {
		counter.Add(1)
		return []byte("hello world"), []byte("audit"), nil
	}, 0, nil, &IdentityCacheOptions{CacheTimeout: time.Minute})
	defer c.Close()

	start := time.Now()
	id, audit, err := c.Identity(nil)
	require.NoError(t, err)
	require.Less(t, time.Since(start), 10*time.Second)
	require.Equal(t, view.Identity([]byte("hello world")), id)
	require.Equal(t, []byte("audit"), audit)
	require.Equal(t, int32(1), counter.Load())
}

// Close must not return while the provisioning goroutine can still cache an identity. The
// goroutine is held in a backend call while Close runs. In a synctest bubble synctest.Wait
// returns once it is blocked there and Close is blocked waiting for it, so the test checks
// that Close has not returned; a Close that does not wait has returned by then. The held call
// is then released with the cache drained, and nothing may reach the cache or the backend
// after Close returns.
func TestIdentityCacheCloseWaitsForProvisioning(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		held := make(chan struct{})
		release := make(chan struct{})
		// Releases the held call if the test fails before it does, so the bubble can end.
		t.Cleanup(func() {
			select {
			case <-release:
			default:
				close(release)
			}
		})
		c := NewIdentityCache(func(_ *driver.IdentityOptions) (view.Identity, []byte, error) {
			// The first call fills the one-slot cache; the second is held.
			calls++
			if calls == 2 {
				close(held)
				<-release
			}
			return []byte("hello world"), []byte("audit"), nil
		}, 1, nil, &IdentityCacheOptions{CacheTimeout: time.Minute})

		// Starts provisioning and takes the cached identity, freeing the slot.
		_, _, err := c.Identity(nil)
		require.NoError(t, err)
		<-held

		closed := make(chan struct{})
		go func() {
			c.Close()
			close(closed)
		}()
		synctest.Wait()
		select {
		case <-closed:
			t.Fatal("Close returned while the provisioning goroutine was still in the backend")
		default:
		}

		// Drain what the goroutine cached before it was held, then let the held call finish.
		for c.CacheSize() > 0 {
			_, _, err := c.Identity(nil)
			require.NoError(t, err)
		}
		close(release)
		<-closed

		// The held call's identity may have been cached before Close returned: its send and
		// the cancellation are both ready once it is released. Nothing changes afterwards.
		size, n := c.CacheSize(), calls
		synctest.Wait()
		require.Equal(t, size, c.CacheSize(), "nothing is cached after Close returns")
		require.Equal(t, n, calls, "no backend call starts after Close returns")
	})
}

// Close is safe before any Identity call, more than once, and concurrently with Identity.
func TestIdentityCacheCloseIdempotent(t *testing.T) {
	t.Parallel()

	never := NewIdentityCache(func(_ *driver.IdentityOptions) (view.Identity, []byte, error) {
		return []byte("id"), nil, nil
	}, 1, nil, nil)
	never.Close()
	never.Close()

	c := NewIdentityCache(func(_ *driver.IdentityOptions) (view.Identity, []byte, error) {
		return []byte("id"), nil, nil
	}, 1, nil, &IdentityCacheOptions{CacheTimeout: time.Millisecond})
	// The goroutines send their errors back, so require runs on the test's own goroutine.
	const n = 4
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			_, _, err := c.Identity(nil)
			errs <- err
		})
		wg.Go(c.Close)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	c.Close()
}
