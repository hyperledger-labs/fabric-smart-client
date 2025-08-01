/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"github.com/hyperledger/fabric-lib-go/common/metrics"
)

type CacheMetrics interface {
	IncrementGet()
	IncrementPut()
	IncrementUpdate()
	IncrementDelete()
	IncrementHit()
	IncrementMiss()
	SetSize(n int)
	SetDataSize(n int)
}

type noCacheMetrics struct{}

func (n *noCacheMetrics) IncrementGet()     {}
func (n *noCacheMetrics) IncrementPut()     {}
func (n *noCacheMetrics) IncrementUpdate()  {}
func (n *noCacheMetrics) IncrementDelete()  {}
func (n *noCacheMetrics) IncrementHit()     {}
func (n *noCacheMetrics) IncrementMiss()    {}
func (n *noCacheMetrics) SetSize(_ int)     {}
func (n *noCacheMetrics) SetDataSize(_ int) {}

type cacheMetrics struct {
	// Total number of get operations
	GetCounter metrics.Counter
	// Total number of put operations
	PutCounter metrics.Counter
	// Total number of updates operations
	UpdateCounter metrics.Counter
	// Total number of delete operations
	DeleteCounter metrics.Counter
	// Total number of successful cache lookups (hits)
	HitCounter metrics.Counter
	// Total number of cache lookups that did not find a matching key (misses)
	MissCounter metrics.Counter
	// Number of entries in cache
	Size metrics.Gauge
	// Total size in bytes of actual data stored in the cache
	DataSize metrics.Gauge
}

func NewCacheMetrics(cacheName string, p metrics.Provider) *cacheMetrics {
	return &cacheMetrics{
		GetCounter: p.NewCounter(metrics.CounterOpts{
			Namespace: "Cache",
			Name:      cacheName + "_get_total",
			Help:      "Total number of get operations",
		}),
		PutCounter: p.NewCounter(metrics.CounterOpts{
			Namespace: "Cache",
			Name:      cacheName + "_put_total",
			Help:      "Total number of put operations",
		}),
		UpdateCounter: p.NewCounter(metrics.CounterOpts{
			Namespace: "Cache",
			Name:      cacheName + "_update_total",
			Help:      "Total number of update operations",
		}),
		DeleteCounter: p.NewCounter(metrics.CounterOpts{
			Namespace: "Cache",
			Name:      cacheName + "_delete_total",
			Help:      "Total number of delete operations",
		}),
		HitCounter: p.NewCounter(metrics.CounterOpts{
			Namespace: "Cache",
			Name:      cacheName + "_hit_total",
			Help:      "Total number of cache hits",
		}),
		MissCounter: p.NewCounter(metrics.CounterOpts{
			Namespace: "Cache",
			Name:      cacheName + "_miss_total",
			Help:      "Total number of cache misses",
		}),
		Size: p.NewGauge(metrics.GaugeOpts{
			Namespace: "Cache",
			Name:      cacheName + "_entry_count",
			Help:      "Number of entries in cache",
		}),
		DataSize: p.NewGauge(metrics.GaugeOpts{
			Namespace: "Cache",
			Name:      cacheName + "_data_size_bytes",
			Help:      "Total size in bytes of actual data stored in the cache",
		}),
	}
}

func (c *cacheMetrics) IncrementGet() {
	c.GetCounter.Add(1)
}

func (c *cacheMetrics) IncrementPut() {
	c.PutCounter.Add(1)
}

func (c *cacheMetrics) IncrementUpdate() {
	c.UpdateCounter.Add(1)
}

func (c *cacheMetrics) IncrementDelete() {
	c.DeleteCounter.Add(1)
}

func (c *cacheMetrics) IncrementHit() {
	c.HitCounter.Add(1)
}

func (c *cacheMetrics) IncrementMiss() {
	c.MissCounter.Add(1)
}

func (c *cacheMetrics) SetSize(n int) {
	c.Size.Set(float64(n))
}

func (c *cacheMetrics) SetDataSize(n int) {
	c.DataSize.Set(float64(n))
}
