/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common4 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

func NewKeyValueStore(dbs *common3.RWDB, tables common4.TableNames) (*KeyValueStore, error) {
	return buildKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS), nil
}

// KeyValueStore is the Postgres key-value store. CreateSchema reorders a
// primary key of (pkey, ns) to (ns, pkey) after the table exists.
type KeyValueStore struct {
	*common4.KeyValueStore

	writeDB *sql.DB
	table   string
}

func (db *KeyValueStore) CreateSchema() error {
	if err := db.KeyValueStore.CreateSchema(); err != nil {
		return err
	}
	return ReorderPrimaryKey(db.writeDB, db.table)
}

type KeyValueStoreNotifier struct {
	*KeyValueStore
	*Notifier
}

func (db *KeyValueStoreNotifier) Close() error {
	return errors.Join(
		db.KeyValueStore.Close(),
		db.Notifier.Close(),
	)
}

func (db *KeyValueStoreNotifier) CreateSchema() error {
	if err := db.KeyValueStore.CreateSchema(); err != nil {
		return err
	}
	return db.Notifier.CreateSchema()
}

func buildKeyValueStore(readDB, writeDB *sql.DB, table string) *KeyValueStore {
	errorWrapper := &ErrorMapper{}

	return &KeyValueStore{
		KeyValueStore: common4.NewKeyValueStore(writeDB, readDB, table, errorWrapper),
		writeDB:       writeDB,
		table:         table,
	}
}
