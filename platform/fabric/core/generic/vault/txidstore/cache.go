/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txidstore

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type Entry = txidstore.Entry[fdriver.ValidationCode]

type cache interface {
	Get(key string) (*Entry, bool)
	Add(key string, value *Entry)
	Delete(key string)
}

type Logger interface {
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Errorf(template string, args ...interface{})
}

type cachedStore struct {
	*txidstore.CachedStore[fdriver.ValidationCode]
	backed fdriver.TXIDStore
}

type notCachedStore struct {
	*txidstore.NotCachedStore[fdriver.ValidationCode]
	backed fdriver.TXIDStore
}

func (s *notCachedStore) GetLastTxID() (string, error) {
	return s.backed.GetLastTxID()
}

func NewNoCache(backed fdriver.TXIDStore) *notCachedStore {
	return &notCachedStore{
		NotCachedStore: txidstore.NewNoCache[fdriver.ValidationCode](backed),
		backed:         backed,
	}
}

func NewCache(backed fdriver.TXIDStore, cache cache, logger Logger) *cachedStore {
	return &cachedStore{
		CachedStore: txidstore.NewCache[fdriver.ValidationCode](backed, cache, logger),
		backed:      backed,
	}
}

func (s *cachedStore) GetLastTxID() (string, error) {
	return s.backed.GetLastTxID()
}
