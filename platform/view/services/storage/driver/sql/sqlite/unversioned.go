/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/notifier"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

type KeyValueStore struct {
	*sqlcommon.KeyValueStore

	readDB  *sql.DB
	writeDB sqlcommon.WriteDB
	table   string
}

// CreateSchema creates the key-value table and reorders a primary key of
// (pkey, ns) to (ns, pkey). The staging table uses the KVS columns: pkey is
// BYTEA and val is BYTEA NOT NULL.
func (db *KeyValueStore) CreateSchema() error {
	if err := db.KeyValueStore.CreateSchema(); err != nil {
		return err
	}
	return ReorderPrimaryKey(db.readDB, db.writeDB, db.table, "ns TEXT NOT NULL, pkey BYTEA NOT NULL, val BYTEA NOT NULL DEFAULT ''", "ns, pkey, val")
}

func NewKeyValueStore(dbs *drivercommon.RWDB, tables sqlcommon.TableNames) (*KeyValueStore, error) {
	return buildKeyValueStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.KVS), nil
}

func NewKeyValueStoreNotifier(dbs *drivercommon.RWDB, table string) (*notifier.UnversionedPersistenceNotifier, error) {
	return notifier.NewUnversioned(buildKeyValueStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), table)), nil
}

func buildKeyValueStore(readDB *sql.DB, writeDB sqlcommon.WriteDB, table string) *KeyValueStore {
	var wrapper driver.SQLErrorWrapper = &ErrorMapper{}
	return &KeyValueStore{
		KeyValueStore: sqlcommon.NewKeyValueStore(writeDB, readDB, table, wrapper),
		readDB:        readDB,
		writeDB:       writeDB,
		table:         table,
	}
}
