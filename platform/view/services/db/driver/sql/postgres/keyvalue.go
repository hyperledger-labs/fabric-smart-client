/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
)

type KeyValueStore struct {
	*common.KeyValueStore

	table        string
	ci           common2.CondInterpreter
	errorWrapper driver2.SQLErrorWrapper
}

func (db *KeyValueStore) SetStates(ns driver.Namespace, kvs map[driver.PKey]driver.UnversionedValue) map[driver.PKey]error {
	encoded := make(map[driver.PKey]driver.UnversionedValue, len(kvs))
	decodeMap := make(map[driver.PKey]driver.PKey, len(kvs))
	for k, v := range kvs {
		enc := encode(k)
		encoded[enc] = v
		decodeMap[enc] = k
	}

	errs := db.SetStatesWithTx(db.Txn, ns, encoded)
	decodedErrs := make(map[driver.PKey]error, len(errs))
	for k, err := range errs {
		decodedErrs[decodeMap[k]] = err
	}
	return decodedErrs
}

func (db *KeyValueStore) SetStateWithTx(tx *sql.Tx, ns driver.Namespace, pkey driver.PKey, value driver.UnversionedValue) error {
	if errs := db.SetStatesWithTx(tx, ns, map[driver.PKey]driver.UnversionedValue{encode(pkey): value}); errs != nil {
		return errs[encode(pkey)]
	}
	return nil
}

func (db *KeyValueStore) GetStateRangeScanIterator(ctx context.Context, ns driver.Namespace, startKey, endKey string) (iterators.Iterator[*driver.UnversionedRead], error) {
	return decodeUnversionedReadIterator(db.KeyValueStore.GetStateRangeScanIterator(ctx, ns, encode(startKey), encode(endKey)))
}

func (db *KeyValueStore) GetStateSetIterator(ctx context.Context, ns driver.Namespace, keys ...driver.PKey) (iterators.Iterator[*driver.UnversionedRead], error) {
	encoded := make([]driver.PKey, len(keys))
	for i, k := range keys {
		encoded[i] = encode(k)
	}
	return decodeUnversionedReadIterator(db.KeyValueStore.GetStateSetIterator(ctx, ns, encoded...))
}

func NewKeyValueStore(dbs *common3.RWDB, tables common.TableNames) (*KeyValueStore, error) {
	return newKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS), nil
}

type KeyValueStoreNotifier struct {
	*KeyValueStore
	*Notifier
}

func (db *KeyValueStoreNotifier) CreateSchema() error {
	if err := db.KeyValueStore.CreateSchema(); err != nil {
		return err
	}
	return db.Notifier.CreateSchema()
}

func newKeyValueStore(readDB, writeDB *sql.DB, table string) *KeyValueStore {
	ci := NewConditionInterpreter()
	errorWrapper := &errorMapper{}
	return &KeyValueStore{
		KeyValueStore: common.NewKeyValueStore(readDB, writeDB, table, errorWrapper, ci),
		table:         table,
		ci:            ci,
		errorWrapper:  errorWrapper,
	}
}
