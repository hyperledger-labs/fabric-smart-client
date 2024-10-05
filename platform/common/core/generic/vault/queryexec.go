/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// this file contains all structs that perform DB access. They
// differ in terms of the results that they return. They are both
// created with the assumption that a read lock on the vault is held.
// The lock is released when Done is called.

type directQueryExecutor[V driver.ValidationCode] struct {
	vault *Vault[V]
}

func (q *directQueryExecutor[V]) GetState(namespace driver.Namespace, key driver.PKey) (driver.RawValue, error) {
	//logger.Debugf("Get State [%s,%s]", namespace, key)
	vv, err := q.vault.store.GetState(namespace, key)
	//logger.Debugf("Got State [%s,%s] -> [%v]", namespace, key, hash.Hashable(v).String())
	return vv.Raw, err
}

func (q *directQueryExecutor[V]) GetStateRangeScanIterator(namespace driver.Namespace, startKey, endKey driver.PKey) (VersionedResultsIterator, error) {
	return q.vault.store.GetStateRangeScanIterator(namespace, startKey, endKey)
}

func (q *directQueryExecutor[V]) GetStateMetadata(namespace driver.Namespace, key driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	m, version, err := q.vault.store.GetStateMetadata(namespace, key)
	if err != nil {
		return nil, nil, err
	}
	return m, version, nil
}

func (q *directQueryExecutor[V]) Done() {
	q.vault.counter.Dec()
	q.vault.storeLock.RUnlock()
}

type interceptorQueryExecutor[V driver.ValidationCode] struct {
	*Vault[V]
}

func (i *interceptorQueryExecutor[V]) Done() {
	i.counter.Dec()
	i.storeLock.RUnlock()
}

func (i *interceptorQueryExecutor[V]) GetStateMetadata(namespace driver.Namespace, key driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	m, version, err := i.store.GetStateMetadata(namespace, key)
	if err != nil {
		return nil, nil, err
	}
	return m, version, nil
}

func (i *interceptorQueryExecutor[V]) GetState(namespace driver.Namespace, key driver.PKey) (VersionedValue, error) {
	return i.store.GetState(namespace, key)
}
