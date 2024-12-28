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

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
)

func TestTimeoutSimple(t *testing.T) {
	mu := sync.RWMutex{}
	allEvicted := make(map[int]string)
	done := make(chan struct{})

	c := cache.NewTimeoutCache(2*time.Second, func(evicted map[int]string) {
		mu.Lock()
		collections.CopyMap(allEvicted, evicted)
		mu.Unlock()
		done <- struct{}{}
	})

	c.Put(1, "a")
	c.Put(2, "b")
	c.Put(3, "c")
	c.Put(4, "d")
	c.Put(5, "e")

	mu.RLock()
	assert.Equal(0, len(allEvicted))
	assert.Equal(5, c.Len())
	v, _ := c.Get(1)
	assert.Equal("a", v)
	mu.RUnlock()
	<-done

	mu.RLock()
	time.Sleep(3 * time.Second)
	assert.Equal(map[int]string{1: "a", 2: "b", 3: "c", 4: "d", 5: "e"}, allEvicted)
	assert.Equal(0, c.Len())
	_, ok := c.Get(1)
	assert.False(ok)
	mu.RUnlock()
}

func TestTimeoutParallel(t *testing.T) {
	evictedCount := atomic.Int32{}
	c := cache.NewTimeoutCache(2*time.Second, func(evicted map[int]string) { evictedCount.Add(int32(len(evicted))) })

	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func(i int) {
			c.Put(i, fmt.Sprintf("item-%d", i))
			wg.Done()
		}(i)
	}
	wg.Wait()
	assert.Equal(100, c.Len())
	assert.Equal(0, int(evictedCount.Load()))

	time.Sleep(3 * time.Second)

	assert.Equal(0, c.Len())
	assert.Equal(100, int(evictedCount.Load()))
}
