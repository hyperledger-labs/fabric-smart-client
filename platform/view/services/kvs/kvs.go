/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import (
	"encoding/json"
	"path/filepath"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var (
	logger = flogging.MustGetLogger("view-sdk.kvs")
	kvs    = &KVS{}
)

const (
	cacheSizeConfigKey       = "fsc.kvs.cache.size"
	persistenceOptsConfigKey = "fsc.kvs.persistence.opts"
)

type KVS struct {
	namespace string
	store     driver.Persistence

	putMutex sync.RWMutex
	cache    cache
}

type cache interface {
	Get(key string) (interface{}, bool)
	Add(key string, value interface{})
	Delete(key string)
}

type Opts struct {
	Path string
}

func New(driverName, namespace string, sp view.ServiceProvider) (*KVS, error) {
	opts := &Opts{}
	err := view.GetConfigService(sp).UnmarshalKey(persistenceOptsConfigKey, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting opts for vault")
	}
	path := filepath.Join(opts.Path, namespace)
	persistence, err := db.Open(driverName, path)
	if err != nil {
		return nil, errors.WithMessagef(err, "no driver found for [%s]", driverName)
	}

	cacheSize, err := cacheSizeFromConfig(sp)
	if err != nil {
		return nil, errors.Wrapf(err, "failed loading cache size from configuration")
	}

	logger.Debugf("opening kvs with namespace=`%s` and cacheSize=`%d` at [%s]", namespace, cacheSize, path)

	return &KVS{
		namespace: namespace,
		store:     persistence,
		cache:     secondcache.New(cacheSize),
	}, nil
}

var defaultCacheSize = 100

// cacheSizeFromConfig returns the KVS cache size from current configuration.
// Returns defaultCacheSize, if no configuration found.
// Returns an error and defaultCacheSize, if the loaded value from configuration is invalid (must be >= 0).
func cacheSizeFromConfig(sp view.ServiceProvider) (int, error) {
	configService := view.GetConfigService(sp)

	if !configService.IsSet(cacheSizeConfigKey) {
		// no cache size configure, let's use default
		return defaultCacheSize, nil
	}

	cacheSize := configService.GetInt(cacheSizeConfigKey)
	if cacheSize < 0 {
		return defaultCacheSize, errors.Errorf("invalid cache size configuration: expect value >= 0, actual %d", cacheSize)
	}
	return cacheSize, nil
}

func (o *KVS) Exists(id string) bool {
	// is in cache?
	o.putMutex.RLock()
	v, ok := o.cache.Get(id)
	if ok {
		o.putMutex.RUnlock()
		return v != nil && len(v.([]byte)) != 0
	}
	o.putMutex.RUnlock()

	// get from store
	o.putMutex.Lock()
	defer o.putMutex.Unlock()

	// is in cache, first?
	v, ok = o.cache.Get(id)
	if ok {
		return v != nil && len(v.([]byte)) != 0
	}
	// get from store and store in cache
	raw, err := o.store.GetState(o.namespace, id)
	if err != nil {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("failed getting state [%s,%s]", o.namespace, id)
		}
		o.cache.Delete(id)
		return false
	}
	o.cache.Add(id, raw)
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("state [%s,%s] exists [%v]", o.namespace, id, len(raw) != 0)
	}

	return len(raw) != 0
}

func (o *KVS) Put(id string, state interface{}) error {
	o.putMutex.Lock()
	defer o.putMutex.Unlock()

	raw, err := json.Marshal(state)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal state with id [%s]", id)
	}

	err = o.store.BeginUpdate()
	if err != nil {
		return errors.WithMessagef(err, "begin update for id [%s] failed", id)
	}

	err = o.store.SetState(o.namespace, id, raw)
	if err != nil {
		if err1 := o.store.Discard(); err1 != nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("got error %s; discarding caused %s", err.Error(), err1.Error())
			}
		}

		return errors.Errorf("failed to commit value for id [%s]", id)
	}

	err = o.store.Commit()
	if err != nil {
		return errors.WithMessagef(err, "committing value for id [%s] failed", id)
	}

	o.cache.Add(id, raw)

	return nil
}

func (o *KVS) Get(id string, state interface{}) error {
	o.putMutex.RLock()
	defer o.putMutex.RUnlock()

	var err error
	var raw []byte
	cachedRaw, ok := o.cache.Get(id)
	if cachedRaw != nil && ok {
		raw = cachedRaw.([]byte)
	} else if !ok {
		raw, err = o.store.GetState(o.namespace, id)
		if err != nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("failed retrieving state [%s,%s]", o.namespace, id)
			}
			return errors.Errorf("failed retrieving state [%s,%s]", o.namespace, id)
		}
		if len(raw) == 0 {
			return errors.Errorf("state [%s,%s] does not exist", o.namespace, id)
		}
	}

	if err := json.Unmarshal(raw, state); err != nil {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("failed retrieving state [%s,%s], cannot unmarshal state, error [%s]", o.namespace, id, err)
		}
		return errors.Wrapf(err, "failed retrieving state [%s,%s], cannot unmarshal state", o.namespace, id)
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("got state [%s,%s] successfully", o.namespace, id)
	}
	return nil
}

func (o *KVS) Delete(id string) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("delete state [%s,%s]", o.namespace, id)
	}

	o.putMutex.Lock()
	defer o.putMutex.Unlock()

	err := o.store.BeginUpdate()
	if err != nil {
		return errors.WithMessagef(err, "begin update for id [%s] failed", id)
	}

	err = o.store.DeleteState(o.namespace, id)
	if err != nil {
		if err1 := o.store.Discard(); err1 != nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("got error %s; discarding caused %s", err.Error(), err1.Error())
			}
		}

		return errors.Errorf("failed to commit value for id [%s]", id)
	}

	err = o.store.Commit()
	if err != nil {
		return errors.WithMessagef(err, "committing value for id [%s] failed", id)
	}

	o.cache.Delete(id)

	return nil
}

func (o *KVS) GetByPartialCompositeID(prefix string, attrs []string) (*iteratorConverter, error) {
	partialCompositeKey, err := CreateCompositeKey(prefix, attrs)
	if err != nil {
		return nil, errors.Errorf("failed building composite key [%s]", err)
	}

	startKey := partialCompositeKey
	endKey := partialCompositeKey + string(maxUnicodeRuneValue)

	itr, err := o.store.GetStateRangeScanIterator(o.namespace, startKey, endKey)
	if err != nil {
		return nil, errors.Errorf("store access failure for GetStateRangeScanIterator [%s], ns [%s] range [%s,%s]", err, o.namespace, startKey, endKey)
	}

	return &iteratorConverter{ri: itr}, nil
}

func (o *KVS) Stop() {
	if err := o.store.Close(); err != nil {
		logger.Errorf("failed stopping kvs [%s]", err)
	}
}

type iteratorConverter struct {
	ri   driver.ResultsIterator
	next *driver.Read
}

func (i *iteratorConverter) HasNext() bool {
	var err error
	i.next, err = i.ri.Next()
	if err != nil || i.next == nil {
		return false
	}
	return true
}

func (i *iteratorConverter) Close() error {
	i.ri.Close()
	return nil
}

// Next unmarshals the current state into the given state object.
// It also returns the key of the current state.
func (i *iteratorConverter) Next(state interface{}) (string, error) {
	return i.next.Key, json.Unmarshal(i.next.Raw, state)
}

func GetService(ctx view.ServiceProvider) *KVS {
	s, err := ctx.GetService(kvs)
	if err != nil {
		panic(err)
	}
	return s.(*KVS)
}
