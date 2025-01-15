/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import (
	"encoding/json"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"go.uber.org/zap/zapcore"
)

var (
	logger = logging.MustGetLogger("view-sdk.kvs")
)

const (
	cacheSizeConfigKey       = "fsc.kvs.cache.size"
	persistenceType          = "fsc.kvs.persistence.type"
	persistenceOptsConfigKey = "fsc.kvs.persistence.opts"
	defaultCacheSize         = 100
)

type cache interface {
	Get(key string) (interface{}, bool)
	Add(key string, value interface{})
	Delete(key string)
}

//go:generate counterfeiter -o mock/config_provider.go -fake-name ConfigProvider . ConfigProvider

// ConfigProvider models the DB configuration provider
type ConfigProvider interface {
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// GetInt returns the value associated with the key as an integer
	GetInt(key string) int
}

type Iterator interface {
	HasNext() bool
	Close() error
	Next(state interface{}) (string, error)
}

type KVS struct {
	namespace string
	store     driver.TransactionalUnversionedPersistence

	putMutex sync.RWMutex
	cache    cache
}

// NewWithConfig returns a new KVS instance for the passed namespace using the passed driver and config provider
func NewWithConfig(dbDriver driver.Driver, namespace string, cp ConfigProvider) (*KVS, error) {
	d, err := dbDriver.NewTransactionalUnversioned(namespace, db.NewPrefixConfig(cp, persistenceOptsConfigKey))
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening datasource [%s]", namespace)
	}
	persistence := &db.TransactionalUnversionedPersistence{TransactionalUnversionedPersistence: d}

	cacheSize, err := cacheSizeFromConfig(cp)
	if err != nil {
		return nil, errors.Wrapf(err, "failed loading cache size from configuration")
	}

	logger.Debugf("opening kvs with namespace=`%s` and cacheSize=`%d`", namespace, cacheSize)

	return &KVS{
		namespace: namespace,
		store:     persistence,
		cache:     secondcache.New(cacheSize),
	}, nil
}

func (o *KVS) GetExisting(ids ...string) []string {
	result := make([]string, 0)
	notFound := make([]string, 0)
	// is in cache?
	o.putMutex.RLock()
	for _, id := range ids {
		if v, ok := o.cache.Get(id); !ok {
			notFound = append(notFound, id)
		} else if v != nil && len(v.([]byte)) > 0 {
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
		} else if v != nil && len(v.([]byte)) > 0 {
			result = append(result, id)
		}
	}
	if len(notFound) == 0 {
		return result
	}

	ids = notFound
	// get from store and store in cache
	it, err := o.store.GetStateSetIterator(o.namespace, ids...)
	if err != nil {
		return result
	}
	for v, err := it.Next(); v != nil || err != nil; v, err = it.Next() {
		if err != nil {
			o.cache.Delete(v.Key)
		} else if len(v.Raw) > 0 {
			o.cache.Add(v.Key, v.Raw)
			result = append(result, v.Key)
		} else {
			o.cache.Add(v.Key, v.Raw)
		}
	}

	return result
}

func (o *KVS) Exists(id string) bool {
	return len(o.GetExisting(id)) > 0
}

func (o *KVS) Put(id string, state interface{}) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal state with id [%s]", id)
	}

	if err := utils.NewProbabilisticRetryRunner(3, 200, true).RunWithErrors(func() (bool, error) {
		tx, err := o.store.NewWriteTransaction()
		if err != nil {
			return true, errors.Wrapf(err, "begin update for id [%s] failed", id)
		}
		logger.Debugf("store [%d] bytes into key [%s:%s]", len(raw), o.namespace, id)

		if err := tx.SetState(o.namespace, id, raw); err != nil {
			if err1 := tx.Discard(); err1 != nil {
				logger.Debugf("got error %v; discarding caused %v", err, err1)
			}

			if !errors.HasCause(err, driver.UniqueKeyViolation) {
				return false, errors.Wrapf(err, "failed to commit value for id [%s]", id)
			}
			return true, nil
		}

		if err := tx.Commit(); err != nil {
			if err1 := tx.Discard(); err1 != nil {
				logger.Debugf("got error %v; discarding caused %v", err, err1)
			}
			return false, errors.Wrapf(err, "committing value for id [%s] failed", id)
		}
		return true, nil
	}); err != nil {
		return err
	}

	o.putMutex.Lock()
	defer o.putMutex.Unlock()
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
			return errors.Wrapf(err, "failed retrieving state [%s,%s]", o.namespace, id)
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

	tx, err := o.store.NewWriteTransaction()
	if err != nil {
		return errors.Wrapf(err, "begin update for id [%s] failed", id)
	}
	if err := tx.DeleteState(o.namespace, id); err != nil {
		if err1 := tx.Discard(); err1 != nil {
			logger.Debugf("got error %v; discarding caused %v", err, err1)
		}

		if !errors.HasCause(err, driver.UniqueKeyViolation) {
			return errors.Wrapf(err, "failed to commit value for id [%s]", id)
		}
	} else {
		if err := tx.Commit(); err != nil {
			return errors.Wrapf(err, "committing value for id [%s] failed", id)
		}
	}

	o.putMutex.Lock()
	defer o.putMutex.Unlock()
	o.cache.Delete(id)
	return nil
}

func (o *KVS) GetByPartialCompositeID(prefix string, attrs []string) (Iterator, error) {
	partialCompositeKey, err := CreateCompositeKey(prefix, attrs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed building composite key")
	}

	startKey := partialCompositeKey
	endKey := partialCompositeKey + string(maxUnicodeRuneValue)

	itr, err := o.store.GetStateRangeScanIterator(o.namespace, startKey, endKey)
	if err != nil {
		return nil, errors.Wrapf(err, "store access failure for GetStateRangeScanIterator, ns [%s] range [%s,%s]", o.namespace, startKey, endKey)
	}

	return &it{ri: itr}, nil
}

func (o *KVS) Stop() {
	if err := o.store.Close(); err != nil {
		logger.Errorf("failed stopping kvs [%s]", err)
	}
}

type it struct {
	ri   driver.UnversionedResultsIterator
	next *driver.UnversionedRead
}

func (i *it) HasNext() bool {
	var err error
	i.next, err = i.ri.Next()
	if err != nil || i.next == nil {
		return false
	}
	return true
}

func (i *it) Close() error {
	i.ri.Close()
	return nil
}

// Next unmarshals the current state into the given state object.
// It also returns the key of the current state.
func (i *it) Next(state interface{}) (string, error) {
	return i.next.Key, json.Unmarshal(i.next.Raw, state)
}

// cacheSizeFromConfig returns the KVS cache size from current configuration.
// Returns defaultCacheSize, if no configuration found.
// Returns an error and defaultCacheSize, if the loaded value from configuration is invalid (must be >= 0).
func cacheSizeFromConfig(cp ConfigProvider) (int, error) {
	if !cp.IsSet(cacheSizeConfigKey) {
		// no cache size configure, let's use default
		return defaultCacheSize, nil
	}

	cacheSize := cp.GetInt(cacheSizeConfigKey)
	if cacheSize < 0 {
		return defaultCacheSize, errors.Errorf("invalid cache size configuration: expect value >= 0, actual %d", cacheSize)
	}
	return cacheSize, nil
}
