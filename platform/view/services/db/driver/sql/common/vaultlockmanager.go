/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"math/rand"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/pkg/errors"
)

const (
	maxLockRetries = 4
	backoffDelay   = time.Second
)

func newLock(release func()) *lock {
	return &lock{release: release}
}

type lock struct {
	release func()
}

func (l *lock) Release() error {
	l.release()
	return nil
}

func newLockManager() *vaultLockManager {
	return &vaultLockManager{txLocks: map[driver.TxID]*sync.RWMutex{}}
}

type vaultLockManager struct {
	globalLock sync.RWMutex
	txLocks    map[driver.TxID]*sync.RWMutex
	txLocksMu  sync.Mutex
}

func (db *vaultLockManager) AcquireWLocks(txIDs ...driver.TxID) error {
	logger.Debugf("Acquire locks for [%v]", txIDs)
	for i := 0; i < maxLockRetries; i++ {
		acquired := make([]driver.TxID, 0, len(txIDs))
		db.globalLock.RLock()
		db.txLocksMu.Lock()
		for _, txID := range txIDs {
			if l, ok := db.txLocks[txID]; !ok {
				logger.Debugf("Locked [%s] successfully", txID)
				mu := &sync.RWMutex{}
				db.txLocks[txID] = mu
				mu.Lock()
				acquired = append(acquired, txID)
			} else if l.TryLock() {
				logger.Warnf("Lock entry for [%s] existed. RLocked successfully", txID)
				acquired = append(acquired, txID)
			} else {
				logger.Infof("Failed locking [%s]. Back off, wait and retry...", txID)
				for _, lockedTxID := range acquired {
					db.txLocks[lockedTxID].Unlock()
				}
				break
			}
		}
		db.txLocksMu.Unlock()
		if len(acquired) == len(txIDs) {
			logger.Debugf("Acquired all locks successfully for [%v]", txIDs)
			return nil
		}
		db.globalLock.RUnlock()
		db.backoffDelay()
	}
	return errors.Errorf("failed locking [%v] after %d attempts", txIDs, maxLockRetries)
}

func (db *vaultLockManager) ReleaseWLocks(txIDs ...driver.TxID) {
	logger.Debugf("Releasing locks for [%v]", txIDs)
	defer logger.Debugf("Released locks for [%v]", txIDs)
	db.globalLock.RUnlock()
	db.txLocksMu.Lock()
	defer db.txLocksMu.Unlock()
	for _, txID := range txIDs {
		db.txLocks[txID].Unlock()
		delete(db.txLocks, txID)
	}
}

func (db *vaultLockManager) ReleaseRLocks(txIDs ...driver.TxID) {
	logger.Debugf("Releasing rlocks for [%v]", txIDs)
	defer logger.Debugf("Released rlocks for [%v]", txIDs)
	db.txLocksMu.Lock()
	defer db.txLocksMu.Unlock()
	for _, txID := range txIDs {
		db.txLocks[txID].RUnlock()
		delete(db.txLocks, txID)
	}
}

func (db *vaultLockManager) AcquireTxIDRLock(txID driver.TxID) (driver.VaultLock, error) {
	logger.Debugf("Acquire rlock for [%s]", txID)
	for i := 0; i < maxLockRetries; i++ {
		db.txLocksMu.Lock()
		if l, ok := db.txLocks[txID]; !ok {
			logger.Debugf("RLocked [%s] successfully", txID)
			mu := &sync.RWMutex{}
			db.txLocks[txID] = mu
			mu.RLock()
			db.txLocksMu.Unlock()
			return newLock(func() { db.ReleaseRLocks(txID) }), nil
		} else if l.TryRLock() {
			logger.Warnf("Lock entry for [%s] existed. RLocked successfully", txID)
			db.txLocksMu.Unlock()
			return newLock(func() { db.ReleaseRLocks(txID) }), nil
		} else {
			logger.Infof("Failed rlocking [%s]. Back off, wait and retry...", txID)
			db.txLocksMu.Unlock()
			db.backoffDelay()
		}
	}
	return nil, errors.Errorf("failed locking [%s] after %d attempts", txID, maxLockRetries)
}

func (db *vaultLockManager) backoffDelay() {
	duration := time.Duration(rand.Int63n(int64(time.Second))) + backoffDelay
	logger.Debugf("Sleeping for %v", duration)
	time.Sleep(duration)
}

func (db *vaultLockManager) AcquireGlobalRLock() (driver.VaultLock, error) {
	db.globalLock.Lock()
	return newLock(db.globalLock.Unlock), nil
}
