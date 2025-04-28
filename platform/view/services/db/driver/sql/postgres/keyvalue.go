/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type KeyValueStore struct {
	*common.KeyValueStore

	table        string
	ci           common.Interpreter
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

func (db *KeyValueStore) GetStateRangeScanIterator(ns driver.Namespace, startKey, endKey string) (collections.Iterator[*driver.UnversionedRead], error) {
	return decodeUnversionedReadIterator(db.KeyValueStore.GetStateRangeScanIterator(ns, encode(startKey), encode(endKey)))
}

func (db *KeyValueStore) GetStateSetIterator(ns driver.Namespace, keys ...driver.PKey) (collections.Iterator[*driver.UnversionedRead], error) {
	encoded := make([]driver.PKey, len(keys))
	for i, k := range keys {
		encoded[i] = encode(k)
	}
	return decodeUnversionedReadIterator(db.KeyValueStore.GetStateSetIterator(ns, encoded...))
}

func NewKeyValueStore(opts Opts) (*KeyValueStore, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS), nil
}

type keyValueStoreNotifier struct {
	*KeyValueStore
	*Notifier
}

func (db *keyValueStoreNotifier) CreateSchema() error {
	if err := db.KeyValueStore.CreateSchema(); err != nil {
		return err
	}
	return db.Notifier.CreateSchema()
}

func NewKeyValueStoreNotifier(opts Opts) (*keyValueStoreNotifier, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return &keyValueStoreNotifier{
		KeyValueStore: newKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS),
		Notifier:      NewNotifier(dbs.WriteDB, tables.KVS, opts.DataSource, AllOperations, primaryKey{"ns", identity}, primaryKey{"pkey", decode}),
	}, nil
}

func newKeyValueStore(readDB, writeDB *sql.DB, table string) *KeyValueStore {
	ci := NewInterpreter()
	errorWrapper := &errorMapper{}
	return &KeyValueStore{
		KeyValueStore: common.NewKeyValueStore(readDB, writeDB, table, errorWrapper, ci),
		table:         table,
		ci:            ci,
		errorWrapper:  errorWrapper,
	}
}
