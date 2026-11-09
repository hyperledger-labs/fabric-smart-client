/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type IdentityCacheBackendFunc func(opts *driver.IdentityOptions) (view.Identity, []byte, error)

type identityCacheEntry struct {
	Identity view.Identity
	Audit    []byte
}

type IdentityCacheOptions struct {
	// CacheTimeout is the max wait time for a cached identity before falling back to backend
	// Default is 1 second if not specified
	CacheTimeout time.Duration
}

type IdentityCache struct {
	once   sync.Once
	backed IdentityCacheBackendFunc
	cache  chan identityCacheEntry
	opts   *driver.IdentityOptions
	// Max wait time for cached identity
	cacheTimeout time.Duration
	// cancel stops the background provisioning, and done is closed when it has returned.
	// Both are set by the once.Do that starts provisioning and read after a once.Do, which
	// orders the reads after the write.
	cancel context.CancelFunc
	done   chan struct{}
}

// NewIdentityCache returns a cache that keeps up to size identities from backed ready in
// advance. size must not be negative; zero disables caching.
func NewIdentityCache(backed IdentityCacheBackendFunc, size int, opts *driver.IdentityOptions, cacheOpts *IdentityCacheOptions) *IdentityCache {
	cacheTimeout := 1 * time.Second // default timeout
	if cacheOpts != nil && cacheOpts.CacheTimeout > 0 {
		cacheTimeout = cacheOpts.CacheTimeout
	}

	ci := &IdentityCache{
		backed:       backed,
		cache:        make(chan identityCacheEntry, size),
		opts:         opts,
		cacheTimeout: cacheTimeout,
	}

	return ci
}

// Close stops the background identity provisioning and returns once it has stopped, so no
// identity is cached and no backend call starts after Close returns. Identities already in the
// cache are still served. It should be called when the IdentityCache is no longer needed, to
// release the provisioning goroutine. Close is safe to call more than once.
func (c *IdentityCache) Close() {
	// Waits for a provisioning start in progress, and keeps a later Identity call from
	// starting one.
	c.once.Do(func() {})
	if c.cancel == nil {
		return
	}
	c.cancel()
	<-c.done
}

// CacheSize returns the current number of cached identities available in the cache.
func (c *IdentityCache) CacheSize() int {
	return len(c.cache)
}

// Identity returns an identity for opts. Only a request without options is served from the
// cache; the first such request starts the background provisioning. A zero-sized cache is
// never filled, so it serves every request from the backend directly.
func (c *IdentityCache) Identity(opts *driver.IdentityOptions) (view.Identity, []byte, error) {
	if opts != nil || cap(c.cache) == 0 {
		return c.fetchIdentityFromBackend(opts)
	}

	c.once.Do(func() {
		var backgroundCtx context.Context
		backgroundCtx, c.cancel = context.WithCancel(context.Background())
		c.done = make(chan struct{})
		go c.provisionIdentities(backgroundCtx)
	})

	logger.Debugf("fetching identity from cache...")

	return c.fetchIdentityFromCache(opts)
}

func (c *IdentityCache) fetchIdentityFromCache(opts *driver.IdentityOptions) (view.Identity, []byte, error) {
	var identity view.Identity
	var audit []byte

	var start time.Time

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		start = time.Now()
	}

	timeout := time.NewTimer(c.cacheTimeout)
	defer timeout.Stop()

	select {

	case entry := <-c.cache:
		identity = entry.Identity
		audit = entry.Audit

		logger.Debugf("fetching identity from cache [%s][%d] took %v", identity, len(audit), logging.Since(start))

	case <-timeout.C:
		id, a, err := c.backed(opts)
		if err != nil {
			return nil, nil, err
		}
		identity = id
		audit = a

		logger.Debugf("fetching identity from backend after a timeout [%s][%d] took %v", identity, len(audit), logging.Since(start))
	}

	return identity, audit, nil
}

func (c *IdentityCache) fetchIdentityFromBackend(opts *driver.IdentityOptions) (view.Identity, []byte, error) {
	logger.Debugf("fetching identity from backend")
	id, audit, err := c.backed(opts)
	if err != nil {
		return nil, nil, err
	}
	logger.Debugf("fetch identity from backend done [%s][%d]", id, len(audit))

	return id, audit, nil
}

func (c *IdentityCache) provisionIdentities(ctx context.Context) {
	defer close(c.done)
	count := 0
	for {
		id, audit, err := c.backed(c.opts)
		if err != nil {
			logger.Errorf("failed to provision identity [%s]", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}

			continue
		}
		logger.DebugfContext(ctx, "generated new idemix identity [%d]", count)
		select {
		case c.cache <- identityCacheEntry{Identity: id, Audit: audit}:
			count++
		case <-ctx.Done():
			return
		}
	}
}
