/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
)

const (
	evictionTimeout = 10 * time.Millisecond
	timeout         = 10 * evictionTimeout
	tick            = evictionTimeout / 4
)

func TestTimeoutSimple(t *testing.T) {
	t.Parallel()
	var mu sync.RWMutex
	allEvicted := make(map[int]string)

	input := map[int]string{1: "a", 2: "b", 3: "c", 4: "d", 5: "e"}

	c := cache.NewTimeoutCache(evictionTimeout, func(evicted map[int]string) {
		mu.Lock()
		collections.CopyMap(allEvicted, evicted)
		mu.Unlock()
	})

	for k, v := range input {
		c.Put(k, v)
	}

	mu.RLock()
	assert.Empty(t, allEvicted)
	assert.Equal(t, 5, c.Len())
	for k, expected := range input {
		actual, _ := c.Get(k)
		assert.Equal(t, expected, actual)
	}
	mu.RUnlock()

	assert.EventuallyWithT(t, func(a *assert.CollectT) {
		// eventually our cache is empty again due to eviction
		assert.Equal(a, 0, c.Len())
	}, timeout, tick)

	mu.RLock()
	assert.Equal(t, input, allEvicted)
	mu.RUnlock()

	_, ok := c.Get(1)
	assert.False(t, ok)
}

func TestTimeoutParallel(t *testing.T) {
	t.Parallel()
	numItem := 100

	var evictedCount atomic.Int32
	c := cache.NewTimeoutCache(evictionTimeout, func(evicted map[int]string) { evictedCount.Add(int32(len(evicted))) })

	var wg sync.WaitGroup
	wg.Add(numItem)
	for i := 0; i < numItem; i++ {
		go func(i int) {
			c.Put(i, fmt.Sprintf("item-%d", i))
			wg.Done()
		}(i)
	}
	wg.Wait()

	// CPU contention may cause items to be evicted during insertion.
	// Verify ranges instead of exact values to avoid flakiness.
	initialLen := c.Len()
	initialEvicted := int(evictedCount.Load())

	assert.GreaterOrEqual(t, initialLen, 0)
	assert.LessOrEqual(t, initialLen, numItem)
	assert.GreaterOrEqual(t, initialEvicted, 0)
	assert.LessOrEqual(t, initialEvicted, numItem)

	assert.EventuallyWithT(t, func(a *assert.CollectT) {
		// eventually our cache is empty again due to eviction
		assert.Equal(a, 0, c.Len())
	}, timeout, tick)

	// once empty evictedCount should match the number of items we cached before
	assert.Equal(t, numItem, int(evictedCount.Load()))
}
