/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txidstore

import (
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type cache interface {
	Get(key string) (interface{}, bool)
	Add(key string, value interface{})
}

type txidStore interface {
	fdriver.TXIDStore
	Get(txid string) (fdriver.ValidationCode, string, error)
	Set(txID string, code fdriver.ValidationCode, message string) error
}

type Cache struct {
	backed txidStore
	cache  cache
}

func NewCache(backed txidStore, cache cache) *Cache {
	return &Cache{backed: backed, cache: cache}
}

func (s *Cache) Get(txID string) (fdriver.ValidationCode, string, error) {
	// first cache
	if val, ok := s.cache.Get(txID); ok {
		return val.(fdriver.ValidationCode), "", nil
	}
	// then backed
	vs, msg, err := s.backed.Get(txID)
	if err != nil {
		return vs, "", err
	}
	s.cache.Add(txID, vs)
	return vs, msg, nil
}

func (s *Cache) Set(txID string, code fdriver.ValidationCode, message string) error {
	if err := s.backed.Set(txID, code, message); err != nil {
		return err
	}
	s.cache.Add(txID, code)
	return nil
}

func (s *Cache) GetLastTxID() (string, error) {
	return s.backed.GetLastTxID()
}

func (s *Cache) Iterator(pos interface{}) (fdriver.TxidIterator, error) {
	return s.backed.Iterator(pos)
}
