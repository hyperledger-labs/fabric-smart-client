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
	Get(txid string) (fdriver.ValidationCode, error)
	Set(txid string, code fdriver.ValidationCode) error
}

type Cache struct {
	backed txidStore
	cache  cache
}

func NewCache(backed txidStore, cache cache) *Cache {
	return &Cache{backed: backed, cache: cache}
}

func (s *Cache) Get(txid string) (fdriver.ValidationCode, error) {
	// first cache
	if val, ok := s.cache.Get(txid); ok {
		return val.(fdriver.ValidationCode), nil
	}
	// then backed
	vs, err := s.backed.Get(txid)
	if err != nil {
		return vs, err
	}
	s.cache.Add(txid, vs)
	return vs, nil
}

func (s *Cache) Set(txid string, code fdriver.ValidationCode) error {
	if err := s.backed.Set(txid, code); err != nil {
		return err
	}
	s.cache.Add(txid, code)
	return nil
}

func (s *Cache) GetLastTxID() (string, error) {
	return s.backed.GetLastTxID()
}

func (s *Cache) Iterator(pos interface{}) (fdriver.TxidIterator, error) {
	return s.backed.Iterator(pos)
}
