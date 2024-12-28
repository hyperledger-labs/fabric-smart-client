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

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
)

func TestLRUSimple(t *testing.T) {
	allEvicted := make(map[int]string)

	c := cache.NewLRUCache(3, 2, func(evicted map[int]string) { collections.CopyMap(allEvicted, evicted) })

	c.Put(1, "a")
	c.Put(2, "b")
	c.Put(3, "c")
	c.Put(4, "d")
	c.Put(5, "e")

	assert.Equal(0, len(allEvicted))
	assert.Equal(5, c.Len())
	v, _ := c.Get(1)
	assert.Equal("a", v)

	c.Put(6, "f")
	assert.Equal(map[int]string{1: "a", 2: "b", 3: "c"}, allEvicted)
	assert.Equal(3, c.Len())
	_, ok := c.Get(1)
	assert.False(ok)
}

func TestLRUSameKey(t *testing.T) {
	allEvicted := make(map[int]string)

	c := cache.NewLRUCache(3, 2, func(evicted map[int]string) { collections.CopyMap(allEvicted, evicted) })

	c.Put(1, "a")
	c.Put(2, "b")
	c.Put(3, "c")
	c.Put(1, "d")
	c.Put(1, "e")
	c.Put(1, "f")
	assert.Equal(0, len(allEvicted))
	assert.Equal(3, c.Len())
	v, _ := c.Get(1)
	assert.Equal("f", v)

	c.Put(4, "g")
	c.Put(5, "h")
	assert.Equal(0, len(allEvicted))
	assert.Equal(5, c.Len())
	v, _ = c.Get(1)
	assert.Equal("f", v)

	c.Put(6, "i")
	assert.Equal(map[int]string{1: "f", 2: "b", 3: "c"}, allEvicted)
	assert.Equal(3, c.Len())
	_, ok := c.Get(1)
	assert.False(ok)

	allEvicted = map[int]string{}

	c.Put(1, "j")
	c.Put(2, "k")

	assert.Equal(0, len(allEvicted))
	assert.Equal(5, c.Len())
	v, _ = c.Get(4)
	assert.Equal("g", v)

	c.Put(3, "l")

	assert.Equal(map[int]string{4: "g", 5: "h", 6: "i"}, allEvicted)
	assert.Equal(3, c.Len())
	v, _ = c.Get(1)
	assert.Equal("j", v)
}

func TestLRUParallel(t *testing.T) {
	evictedCount := atomic.Int32{}
	c := cache.NewLRUCache(3, 2, func(evicted map[int]string) { evictedCount.Add(int32(len(evicted))) })

	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func(i int) {
			c.Put(i, fmt.Sprintf("item-%d", i))
			wg.Done()
		}(i)
	}
	wg.Wait()

	assert.Equal(4, c.Len())
	assert.Equal(96, int(evictedCount.Load()))
}
