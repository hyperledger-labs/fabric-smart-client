package cache

import "sync"

type sizeFunc[T any] func(T) int

type metricMapCache[K comparable, V any] struct {
	// Reference to the original mapCache
	mapCache *mapCache[K, V]
	// Metrics implementation used to track cache operations
	metrics CacheMetrics
	// Estimation of map cache memory size
	memsize int
	// Estimation of key size
	keySize sizeFunc[K]
	// Estimation of value size
	valSize sizeFunc[V]
	// Lock to update the size
	locksize sync.RWMutex
}

func NewMetricMapCache[K comparable, V any](keySize sizeFunc[K], valSize sizeFunc[V], metrics CacheMetrics) *metricMapCache[K, V] {
	return &metricMapCache[K, V]{
		mapCache: NewMapCache[K, V](),
		metrics:  metrics,
		memsize:  0,
	}
}

func NewMetricSafeMapCache[K comparable, V any, keySize sizeFunc[K], valSize sizeFunc[V]](metrics CacheMetrics) *metricMapCache[K, V] {
	return &metricMapCache[K, V]{
		mapCache: NewSafeMapCache[K, V](),
		metrics:  metrics,
	}
}

func (m *metricMapCache[K, V]) updateMetricSize(entrySize int) {
	m.locksize.Lock()
	defer m.locksize.Unlock()
	m.memsize = m.memsize + entrySize
	m.metrics.SetDataSize(entrySize)
	m.metrics.SetSize(m.mapCache.Len())
}

func (m *metricMapCache[K, V]) Get(key K) (V, bool) {
	m.metrics.IncrementGet()
	val, ok := m.mapCache.Get(key)
	if ok {
		m.metrics.IncrementHit()
	} else {
		m.metrics.IncrementMiss()
	}
	return val, ok
}

func (m *metricMapCache[K, V]) Put(key K, value V) {
	m.mapCache.Put(key, value)
	m.metrics.IncrementPut()
	m.updateMetricSize(m.keySize(key) + m.valSize(value))
}

func (m *metricMapCache[K, V]) Delete(key K) {
	val, ok := m.mapCache.Get(key)
	m.mapCache.Delete(key)
	m.metrics.IncrementDelete()
	if ok {
		m.updateMetricSize(-1 * (m.keySize(key) + m.valSize(val)))
	}
}

func (m *metricMapCache[K, V]) Update(key K, f func(bool, V) (bool, V)) bool {
	val, ok := m.mapCache.Get(key)
	if ok {
		m.updateMetricSize(-1 * (m.keySize(key) + m.valSize(val)))
	}
	ok = m.mapCache.Update(key, f)
	if ok {
		val, _ = m.mapCache.Get(key)
		m.updateMetricSize((m.keySize(key) + m.valSize(val)))
	}
	m.metrics.IncrementUpdate()
	return ok
}
