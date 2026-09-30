/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache/secondcache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

var logger = logging.MustGetLogger()

const (
	cacheSizeConfigKey       = "fsc.kvs.cache.size"
	persistenceType          = "fsc.kvs.persistence.type"
	persistenceOptsConfigKey = "fsc.kvs.persistence.opts"
	DefaultCacheSize         = 100
)

type cache interface {
	Get(key string) (any, bool)
	Add(key string, value any)
	Delete(key string)
}

//go:generate counterfeiter -o mock/config_provider.go -fake-name ConfigProvider . ConfigProvider

// ConfigProvider models the DB configuration provider
type ConfigProvider interface {
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal any) error
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// GetInt returns the value associated with the key as an integer
	GetInt(key string) int
}

// Iterator iterates over the states returned by a KVS scan.
//
// Callers alternate HasNext and Next:
//
//	for it.HasNext() {
//		key, err := it.Next(&state)
//		if err != nil {
//			return err
//		}
//	}
//
// HasNext advances to the next state and reports whether there is one for Next
// to return. When reading from the store fails, HasNext returns true and Next
// returns the failure, so a scan that did not complete is never mistaken for
// one that did. Iteration ends after a failure. The caller must call Close when
// done.
type Iterator interface {
	// HasNext advances the iterator and reports whether Next has a state or a
	// read failure to return.
	HasNext() bool
	// Close releases the underlying store iterator.
	Close() error
	// Next unmarshals the current state into state and returns its key. It
	// returns an error if reading the state from the store failed, if the state
	// cannot be unmarshalled, or if there is no current state because HasNext
	// was not called or returned false.
	Next(state any) (string, error)
}

type KVS struct {
	namespace string
	store     driver.KeyValueStore

	putMutex sync.RWMutex
	cache    cache
}

// New returns a new KVS instance for the passed namespace using the passed driver and config provider
func New(persistence driver.KeyValueStore, namespace string, cacheSize int) (*KVS, error) {
	return &KVS{
		namespace: namespace,
		store:     persistence,
		cache:     secondcache.New(cacheSize),
	}, nil
}

// GetExisting returns the subset of ids whose values are non-empty, looking them up
// in the cache first and in the store for the rest. The lookup is best-effort: if the
// store fails, GetExisting stops and returns the ids found so far. Ids read from the
// store before the failure are cached; the remaining ids are not, so a later call
// queries the store for them again.
func (o *KVS) GetExisting(ctx context.Context, ids ...string) []string {
	result := make([]string, 0)
	notFound := make([]string, 0)
	// is in cache?
	o.putMutex.RLock()
	for _, id := range ids {
		if v, ok := o.cache.Get(id); !ok {
			notFound = append(notFound, id)
		} else if raw, ok := v.([]byte); ok && len(raw) > 0 {
			result = append(result, id)
		}
	}
	if len(notFound) == 0 {
		defer o.putMutex.RUnlock()
		return result
	}
	o.putMutex.RUnlock()

	// get from store
	o.putMutex.Lock()
	defer o.putMutex.Unlock()

	// is in cache, first?
	ids = notFound
	notFound = make([]string, 0)
	for _, id := range ids {
		if v, ok := o.cache.Get(id); !ok {
			notFound = append(notFound, id)
		} else if raw, ok := v.([]byte); ok && len(raw) > 0 {
			result = append(result, id)
		}
	}
	if len(notFound) == 0 {
		return result
	}

	ids = notFound
	// get from store and store in cache
	it, err := o.store.GetStateSetIterator(ctx, o.namespace, ids...)
	if err != nil {
		return result
	}
	defer it.Close()
	for {
		v, err := it.Next()
		if err != nil || v == nil {
			break
		}
		o.cache.Add(v.Key, v.Raw)
		if len(v.Raw) > 0 {
			result = append(result, v.Key)
		}
	}

	return result
}

// Exists reports whether id has a non-empty value. It follows the best-effort
// semantics of GetExisting, so a store failure is reported as false.
func (o *KVS) Exists(ctx context.Context, id string) bool {
	return len(o.GetExisting(ctx, id)) > 0
}

func (o *KVS) Put(ctx context.Context, id string, state any) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal state with id [%s]", id)
	}

	if err := utils.NewProbabilisticRetryRunner(3, 200, true).RunWithErrors(func() (bool, error) {
		err := o.store.SetState(ctx, o.namespace, id, raw)
		return err == nil, err
	}); err != nil {
		return err
	}

	o.putMutex.Lock()
	defer o.putMutex.Unlock()
	o.cache.Add(id, raw)

	return nil
}

func (o *KVS) Get(ctx context.Context, id string, state any) error {
	var err error
	var raw []byte

	// Try to get from cache first (read lock)
	o.putMutex.RLock()
	cachedRaw, ok := o.cache.Get(id)
	//nolint:gocritic // rewriting to switch would obscure the RLock/RUnlock and Lock/Unlock pairing across mixed branches (one branch has two early error returns); the if/else-if reads clearer and is not misleading here.
	if cachedRaw != nil && ok {
		var castOk bool
		raw, castOk = cachedRaw.([]byte)
		if !castOk {
			o.putMutex.RUnlock()
			return errors.Errorf("unexpected cached value type for [%s,%s]", o.namespace, id)
		}
		o.putMutex.RUnlock()
	} else if !ok {
		// Cache miss, need to fetch from store and add to cache
		o.putMutex.RUnlock()

		// Fetch from store
		raw, err = o.store.GetState(ctx, o.namespace, id)
		if err != nil {
			logger.DebugfContext(ctx, "failed retrieving state [%s,%s]", o.namespace, id)
			return errors.Wrapf(err, "failed retrieving state [%s,%s]", o.namespace, id)
		}
		if len(raw) == 0 {
			return errors.Errorf("state [%s,%s] does not exist", o.namespace, id)
		}

		// Add to cache (write lock)
		o.putMutex.Lock()
		o.cache.Add(id, raw)
		o.putMutex.Unlock()
	} else {
		// cachedRaw is nil but ok is true unlock cache
		o.putMutex.RUnlock()
	}

	if err := json.Unmarshal(raw, state); err != nil {
		logger.DebugfContext(ctx, "failed retrieving state [%s,%s], cannot unmarshal state, error [%s]", o.namespace, id, err)
		return errors.Wrapf(err, "failed retrieving state [%s,%s], cannot unmarshal state", o.namespace, id)
	}

	logger.DebugfContext(ctx, "got state [%s,%s] successfully", o.namespace, id)
	return nil
}

func (o *KVS) Delete(ctx context.Context, id string) error {
	logger.DebugfContext(ctx, "delete state [%s,%s]", o.namespace, id)

	if err := o.store.DeleteState(ctx, o.namespace, id); err != nil {
		return err
	}

	o.putMutex.Lock()
	defer o.putMutex.Unlock()
	o.cache.Delete(id)
	return nil
}

// GetByPartialCompositeID returns an Iterator over the states whose keys are
// composite keys, as built by CreateCompositeKey, that start with prefix and attrs.
// The caller must close the returned Iterator.
func (o *KVS) GetByPartialCompositeID(ctx context.Context, prefix string, attrs []string) (Iterator, error) {
	startKey, endKey, err := CreateRangeKeysForPartialCompositeKey(prefix, attrs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed building composite key")
	}

	itr, err := o.store.GetStateRangeScanIterator(ctx, o.namespace, startKey, endKey)
	if err != nil {
		return nil, errors.Wrapf(err, "store access failure for GetStateRangeScanIterator, ns [%s] range [%s,%s]", o.namespace, startKey, endKey)
	}

	return &it{ri: itr}, nil
}

func (o *KVS) Stop() {
	if err := o.store.Close(); err != nil {
		logger.Errorf("failed stopping kvs [%s]", err.Error())
	}
}

type it struct {
	ri   iterators.Iterator[*driver.UnversionedRead]
	next *driver.UnversionedRead
	err  error
	done bool
}

func (i *it) HasNext() bool {
	i.next, i.err = nil, nil
	if i.done {
		return false
	}
	next, err := i.ri.Next()
	switch {
	case err != nil:
		// the store iterator is not read after a failure
		i.err, i.done = err, true
		return true
	case next == nil:
		i.done = true
		return false
	default:
		i.next = next
		return true
	}
}

func (i *it) Close() error {
	i.ri.Close()
	return nil
}

func (i *it) Next(state any) (string, error) {
	if i.err != nil {
		err := i.err
		i.err = nil
		return "", errors.Wrap(err, "failed reading next state")
	}
	if i.next == nil {
		return "", errors.New("no current state")
	}
	return i.next.Key, json.Unmarshal(i.next.Raw, state)
}

// CacheSizeFromConfig returns the KVS cache size from current configuration.
// Returns DefaultCacheSize, if no configuration found.
// Returns an error and DefaultCacheSize, if the loaded value from configuration
// is invalid (must be >= 1). The cache holds a fixed number of slots and needs
// at least one to evict into, and there is no separate way to disable it, so a
// configured 0 is a misconfiguration rather than a request to turn it off.
func CacheSizeFromConfig(cp ConfigProvider) (int, error) {
	if !cp.IsSet(cacheSizeConfigKey) {
		// no cache size configure, let's use default
		return DefaultCacheSize, nil
	}

	cacheSize := cp.GetInt(cacheSizeConfigKey)
	if cacheSize < 1 {
		return DefaultCacheSize, errors.Errorf("invalid cache size configuration: expect value >= 1, actual %d", cacheSize)
	}
	return cacheSize, nil
}
