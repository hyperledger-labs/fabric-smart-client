/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

var logger = logging.MustGetLogger("viewsdk.cached-vault")

type CachedVaultStore interface {
	driver.VaultStore
	Invalidate(txIDs ...driver.TxID)
}

type entry struct {
	Code    driver.TxStatusCode
	Message string
}

type cache interface {
	Get(key string) (*entry, bool)
	Add(key string, value *entry)
	Delete(key string)
}

type cachedStore struct {
	driver.VaultStore
	cache cache
}

type notCachedStore struct {
	driver.VaultStore
}

func (s *notCachedStore) Invalidate(...driver.TxID) {}

func NewCachedVault(backed driver2.VaultPersistence, cacheSize int) CachedVaultStore {
	if cacheSize <= 0 {
		logger.Debugf("txID store without cache selected")
		return &notCachedStore{VaultStore: backed}
	}
	logger.Debugf("creating txID store second cache with size [%d]", cacheSize)
	return &cachedStore{
		VaultStore: backed,
		cache:      secondcache.NewTyped[*entry](cacheSize),
	}
}

func (s *cachedStore) Invalidate(txIDs ...driver.TxID) {
	logger.Debugf("Invalidating cache entry for [%v]", txIDs)

	for _, txID := range txIDs {
		s.cache.Delete(txID)
	}
}

func (s *cachedStore) GetTxStatus(txID driver.TxID) (*driver.TxStatus, error) {
	if e, ok := s.cache.Get(txID); ok && e != nil { // Deleted entries return ok
		logger.Debugf("Found value for [%s] in cache: %v", txID, e.Code)
		return &driver.TxStatus{
			TxID:    txID,
			Code:    e.Code,
			Message: e.Message,
		}, nil
	}

	return s.forceGet(txID)
}

func (s *cachedStore) forceGet(txID driver.TxID) (*driver.TxStatus, error) {
	txStatus, err := s.VaultStore.GetTxStatus(txID)
	if err != nil || txStatus == nil {
		logger.Debugf("Force get returned no value from backed for [%s]", txID)
		return nil, err
	}

	logger.Debugf("Force get returned value [%v] from backed: %v", *txStatus, err)
	s.cache.Add(txID, &entry{Code: txStatus.Code, Message: txStatus.Message})
	return txStatus, nil
}

func (s *cachedStore) Store(txIDs []driver.TxID, writes driver.Writes, metaWrites driver.MetaWrites) error {
	logger.Debugf("Store writes and meta-writes for [%v] into backed and cache", txIDs)
	if err := s.VaultStore.Store(txIDs, writes, metaWrites); err != nil {
		return err
	}

	for _, txID := range txIDs {
		s.cache.Add(txID, &entry{Code: driver.Valid})
	}
	return nil
}

func (s *cachedStore) SetStatuses(code driver.TxStatusCode, message string, txIDs ...driver.TxID) error {
	logger.Debugf("Set value [%v] for [%v] into backed and cache", code, txIDs)
	if err := s.VaultStore.SetStatuses(code, message, txIDs...); err != nil {
		return err
	}

	for _, txID := range txIDs {
		s.cache.Add(txID, &entry{Code: code, Message: message})
	}
	return nil
}
