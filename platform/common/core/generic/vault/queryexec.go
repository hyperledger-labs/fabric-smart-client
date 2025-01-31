/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"

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

func newGlobalLockQueryExecutor(ctx context.Context, vaultStore driver.VaultStore) (*queryExecutor, error) {
	lock, err := vaultStore.AcquireGlobalLock(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to acquire global lock")
	}
	return &queryExecutor{
		vaultStore: vaultStore,
		lock:       lock,
	}, nil
}

func newTxLockQueryExecutor(ctx context.Context, vaultStore driver.VaultStore, txID driver.TxID) (*queryExecutor, error) {
	lock, err := vaultStore.AcquireTxIDRLock(ctx, txID)
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

func (i *queryExecutor) GetStateMetadata(ctx context.Context, namespace driver.Namespace, key driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	return i.vaultStore.GetStateMetadata(ctx, namespace, key)
}

func (i *queryExecutor) GetState(ctx context.Context, namespace driver.Namespace, key driver.PKey) (*VersionedRead, error) {
	return i.vaultStore.GetState(ctx, namespace, key)
}

func (i *queryExecutor) GetStateRangeScanIterator(ctx context.Context, namespace driver.Namespace, startKey, endKey driver.PKey) (VersionedResultsIterator, error) {
	return i.vaultStore.GetStateRange(ctx, namespace, startKey, endKey)
}
