/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestIdentityCache(t *testing.T) { //nolint:paralleltest
	var counter atomic.Int32
	c := NewIdentityCache(func(opts *driver.IdentityOptions) (view.Identity, []byte, error) {
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

func TestIdentityCacheClose(t *testing.T) { //nolint:paralleltest
	var counter atomic.Int32
	c := NewIdentityCache(func(opts *driver.IdentityOptions) (view.Identity, []byte, error) {
		counter.Add(1)
		return []byte("hello world"), []byte("audit"), nil
	}, 10, nil, nil)

	// Trigger cache initialization
	id, audit, err := c.Identity(nil)
	require.NoError(t, err)
	require.Equal(t, view.Identity([]byte("hello world")), id)
	require.Equal(t, []byte("audit"), audit)

	// Wait for cache to fill up
	require.Eventually(t, func() bool {
		return c.CacheSize() >= 10
	}, 2*time.Second, 10*time.Millisecond, "cache should fill up to capacity")

	initialCacheSize := c.CacheSize()
	initialCounter := counter.Load()

	// Close the cache to stop background provisioning
	c.Close()

	// Exhaust all cached identities
	for range initialCacheSize {
		id, audit, err = c.Identity(nil)
		require.NoError(t, err)
		require.Equal(t, view.Identity([]byte("hello world")), id)
		require.Equal(t, []byte("audit"), audit)
	}

	// Verify cache is now empty
	require.Equal(t, 0, c.CacheSize(), "cache should be empty after exhausting all identities")

	// Wait a bit to ensure no new identities are being provisioned
	time.Sleep(100 * time.Millisecond)

	// Verify cache size hasn't increased (background goroutine stopped)
	require.Equal(t, 0, c.CacheSize(), "cache should remain empty after close")

	// Verify counter hasn't increased (no new identities generated)
	require.Equal(t, initialCounter, counter.Load(), "no new identities should be generated after close")

	// But we should still be able to get identities (from backend)
	id, audit, err = c.Identity(nil)
	require.NoError(t, err)
	require.Equal(t, view.Identity([]byte("hello world")), id)
	require.Equal(t, []byte("audit"), audit)

	// Verify this came from backend (counter increased)
	require.Equal(t, initialCounter+1, counter.Load(), "identity should come from backend after cache exhausted")

	// Cache should still be empty (no background provisioning)
	require.Equal(t, 0, c.CacheSize(), "cache should remain empty after fetching from backend")
}
