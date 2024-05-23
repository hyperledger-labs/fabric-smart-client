/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

// this file contains all structs that perform DB access. They
// differ in terms of the results that they return. They are both
// created with the assumption that a read lock on the vault is held.
// The lock is released when Done is called.

type directQueryExecutor[V ValidationCode] struct {
	vault *Vault[V]
}

func (q *directQueryExecutor[V]) GetState(namespace string, key string) ([]byte, error) {
	//logger.Debugf("Get State [%s,%s]", namespace, key)
	vv, err := q.vault.store.GetState(namespace, key)
	//logger.Debugf("Got State [%s,%s] -> [%v]", namespace, key, hash.Hashable(v).String())
	return vv.Raw, err
}

func (q *directQueryExecutor[V]) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (VersionedResultsIterator, error) {
	return q.vault.store.GetStateRangeScanIterator(namespace, startKey, endKey)
}

func (q *directQueryExecutor[V]) GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error) {
	return q.vault.store.GetStateMetadata(namespace, key)
}

func (q *directQueryExecutor[V]) Done() {
	q.vault.counter.Dec()
	q.vault.storeLock.RUnlock()
}

type interceptorQueryExecutor[V ValidationCode] struct {
	*Vault[V]
}

func (i *interceptorQueryExecutor[V]) Done() {
	i.counter.Dec()
	i.storeLock.RUnlock()
}

func (i *interceptorQueryExecutor[V]) GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error) {
	return i.store.GetStateMetadata(namespace, key)
}

func (i *interceptorQueryExecutor[V]) GetState(namespace, key string) (VersionedValue, error) {
	return i.store.GetState(namespace, key)
}
