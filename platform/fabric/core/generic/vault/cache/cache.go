/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/vault"
)

type Entry = vault.Entry[fdriver.ValidationCode]

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

type notCachedStore struct {
	driver.VaultStore
}

func (s *notCachedStore) Invalidate(...driver.TxID) {}

type CachedVaultStore interface {
	driver.VaultStore
	Invalidate(txIDs ...driver.TxID)
}

func NewNoCache(backed driver.VaultStore) CachedVaultStore {
	return &notCachedStore{backed}
}

func NewCache(backed driver.VaultStore, cache cache, logger Logger) CachedVaultStore {
	return &notCachedStore{backed}
}
