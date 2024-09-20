/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txidstore

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
)

type Logger interface {
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Errorf(template string, args ...interface{})
}

type Entry[V driver.ValidationCode] struct {
	ValidationCode    V
	ValidationMessage string
}

type cache[V driver.ValidationCode] interface {
	Get(key string) (*Entry[V], bool)
	Add(key string, value *Entry[V])
	Delete(key string)
}

type txidStore[V driver.ValidationCode] interface {
	Get(txID driver.TxID) (V, string, error)
	Set(txID driver.TxID, code V, message string) error
	SetMultiple(txs []driver.ByNum[V]) error
	Iterator(pos interface{}) (collections.Iterator[*driver.ByNum[V]], error)
}

type CachedStore[V driver.ValidationCode] struct {
	backed txidStore[V]
	cache  cache[V]
	logger Logger
}

type NotCachedStore[V driver.ValidationCode] struct {
	txidStore[V]
}

func (s *NotCachedStore[V]) Invalidate(string) {}

func NewNoCache[V driver.ValidationCode](backed txidStore[V]) *NotCachedStore[V] {
	return &NotCachedStore[V]{txidStore: backed}
}

func NewCache[V driver.ValidationCode](backed txidStore[V], cache cache[V], logger Logger) *CachedStore[V] {
	return &CachedStore[V]{backed: backed, cache: cache, logger: logger}
}

func (s *CachedStore[V]) Invalidate(txID driver.TxID) {
	s.logger.Debugf("Invalidating cache entry for [%s]", txID)
	s.cache.Delete(txID)
}

func (s *CachedStore[V]) Get(txID driver.TxID) (V, string, error) {
	// first cache
	if entry, ok := s.cache.Get(txID); ok && entry != nil { // Deleted entries return ok
		s.logger.Debugf("Found value for [%s] in cache: %v", txID, entry.ValidationCode)
		return entry.ValidationCode, entry.ValidationMessage, nil
	}
	// then backed
	return s.forceGet(txID)
}

func (s *CachedStore[V]) forceGet(txID driver.TxID) (V, string, error) {
	vc, msg, err := s.backed.Get(txID)
	s.logger.Debugf("Got value [%v] from backed: %v", vc, err)
	if err != nil {
		return vc, "", err
	}
	s.cache.Add(txID, &Entry[V]{ValidationCode: vc, ValidationMessage: msg})
	return vc, msg, nil
}

func (s *CachedStore[V]) Set(txID string, code V, message string) error {
	s.logger.Debugf("Set value [%v] for [%s] into backed and cache", code, txID)
	if err := s.backed.Set(txID, code, message); err != nil {
		return err
	}
	s.cache.Add(txID, &Entry[V]{ValidationCode: code, ValidationMessage: message})
	return nil
}

func (s *CachedStore[V]) SetMultiple(txs []driver.ByNum[V]) error {
	s.logger.Debugf("Set values for %d txs into backed and cache", len(txs))
	if err := s.backed.SetMultiple(txs); err != nil {
		return err
	}
	for _, tx := range txs {
		s.cache.Add(tx.TxID, &Entry[V]{ValidationCode: tx.Code, ValidationMessage: tx.Message})
	}
	return nil
}

func (s *CachedStore[V]) Iterator(pos interface{}) (collections.Iterator[*driver.ByNum[V]], error) {
	return s.backed.Iterator(pos)
}
