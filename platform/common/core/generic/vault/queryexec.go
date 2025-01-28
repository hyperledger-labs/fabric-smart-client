/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/pkg/errors"
)

// this file contains all structs that perform DB access. They
// differ in terms of the results that they return. They are both
// created with the assumption that a read lock on the vault is held.
// The lock is released when Done is called.
type queryExecutor struct {
	vaultStore driver.VaultStore
	lock       driver.VaultLock
}

func newGlobalLockQueryExecutor(vaultStore driver.VaultStore) (*queryExecutor, error) {
	lock, err := vaultStore.AcquireGlobalLock()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to acquire global lock")
	}
	return &queryExecutor{
		vaultStore: vaultStore,
		lock:       lock,
	}, nil
}

func newTxLockQueryExecutor(vaultStore driver.VaultStore, txID driver.TxID) (*queryExecutor, error) {
	lock, err := vaultStore.AcquireTxIDRLock(txID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to acquire lock for tx [%s]", txID)
	}
	return &queryExecutor{
		vaultStore: vaultStore,
		lock:       lock,
	}, nil
}

func (i *queryExecutor) Done() {
	_ = i.lock.Release()
}

func (i *queryExecutor) GetStateMetadata(namespace driver.Namespace, key driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	return i.vaultStore.GetStateMetadata(namespace, key)
}

func (i *queryExecutor) GetState(namespace driver.Namespace, key driver.PKey) (*VersionedRead, error) {
	return i.vaultStore.GetState(namespace, key)
}

func (i *queryExecutor) GetStateRangeScanIterator(namespace driver.Namespace, startKey, endKey driver.PKey) (VersionedResultsIterator, error) {
	return i.vaultStore.GetStateRange(namespace, startKey, endKey)
}
