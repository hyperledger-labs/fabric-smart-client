/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txidstore

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
)

type Logger interface {
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Errorf(template string, args ...interface{})
}

type Entry[V vault.ValidationCode] struct {
	ValidationCode    V
	ValidationMessage string
}

type cache[V vault.ValidationCode] interface {
	Get(key string) (*Entry[V], bool)
	Add(key string, value *Entry[V])
	Delete(key string)
}

type txidStore[V vault.ValidationCode] interface {
	Get(txID core.TxID) (V, string, error)
	Set(txID core.TxID, code V, message string) error
	Iterator(pos interface{}) (vault.TxIDIterator[V], error)
}

type CachedStore[V vault.ValidationCode] struct {
	backed txidStore[V]
	cache  cache[V]
	logger Logger
}

type NotCachedStore[V vault.ValidationCode] struct {
	txidStore[V]
}

func (s *NotCachedStore[V]) Invalidate(string) {}

func NewNoCache[V vault.ValidationCode](backed txidStore[V]) *NotCachedStore[V] {
	return &NotCachedStore[V]{txidStore: backed}
}

func NewCache[V vault.ValidationCode](backed txidStore[V], cache cache[V], logger Logger) *CachedStore[V] {
	return &CachedStore[V]{backed: backed, cache: cache, logger: logger}
}

func (s *CachedStore[V]) Invalidate(txID core.TxID) {
	s.logger.Debugf("Invalidating cache entry for [%s]", txID)
	s.cache.Delete(txID)
}

func (s *CachedStore[V]) Get(txID core.TxID) (V, string, error) {
	// first cache
	if entry, ok := s.cache.Get(txID); ok && entry != nil { // Deleted entries return ok
		s.logger.Debugf("Found value for [%s] in cache: %v", txID, entry.ValidationCode)
		return entry.ValidationCode, entry.ValidationMessage, nil
	}
	// then backed
	return s.forceGet(txID)
}

func (s *CachedStore[V]) forceGet(txID core.TxID) (V, string, error) {
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

func (s *CachedStore[V]) Iterator(pos interface{}) (vault.TxIDIterator[V], error) {
	return s.backed.Iterator(pos)
}
