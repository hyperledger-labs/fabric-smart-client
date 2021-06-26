/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
)

// this file contains all structs that perform DB access. They
// differ in terms of the results that they return. They are both
// created with the assumption that a read lock on the vault is held.
// The lock is released when Done is called.

type directQueryExecutor struct {
	vault *Vault
}

func (q *directQueryExecutor) GetState(namespace string, key string) ([]byte, error) {
	logger.Debugf("Get State [%s,%s]", namespace, key)
	v, _, _, err := q.vault.store.GetState(namespace, key)
	logger.Debugf("Got State [%s,%s] -> [%v]", namespace, key, hash.Hashable(v).String())
	return v, err
}

func (q *directQueryExecutor) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (driver.VersionedResultsIterator, error) {
	return q.vault.store.GetStateRangeScanIterator(namespace, startKey, endKey)
}

func (q *directQueryExecutor) GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error) {
	return q.vault.store.GetStateMetadata(namespace, key)
}

func (q *directQueryExecutor) Done() {
	q.vault.counter.Dec()
	q.vault.storeLock.RUnlock()
}

type interceptorQueryExecutor struct {
	*Vault
}

func (i *interceptorQueryExecutor) Done() {
	i.counter.Dec()
	i.storeLock.RUnlock()
}

func (i *interceptorQueryExecutor) GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error) {
	return i.store.GetStateMetadata(namespace, key)
}

func (i *interceptorQueryExecutor) GetState(namespace, key string) ([]byte, uint64, uint64, error) {
	return i.store.GetState(namespace, key)
}
