/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/notifier"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

type KeyValueStore struct {
	*sqlcommon.KeyValueStore

	readDB  *sql.DB
	writeDB sqlcommon.WriteDB
	table   string
}

// kvsColumns declares the key-value table's columns without the primary key,
// for the staging table of a primary-key reorder.
const kvsColumns = "ns TEXT NOT NULL, pkey BYTEA NOT NULL, val BYTEA NOT NULL DEFAULT ''"

// CreateSchema creates the key-value table and reorders a primary key of
// (pkey, ns) to (ns, pkey).
func (db *KeyValueStore) CreateSchema() error {
	if err := db.KeyValueStore.CreateSchema(); err != nil {
		return err
	}
	return ReorderPrimaryKey(db.readDB, db.writeDB, db.table, kvsColumns, "ns, pkey, val")
}

// Steps returns the store's schema steps. Step 1 creates the table if it does
// not exist and reorders a primary key of (pkey, ns) to (ns, pkey).
func (db *KeyValueStore) Steps() []migrate.Step {
	steps := db.KeyValueStore.Steps()
	steps[0].Go = func(ctx context.Context, q migrate.Execer) error {
		return ReorderPrimaryKeyTx(ctx, q, db.table, kvsColumns, "ns, pkey, val")
	}
	return steps
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
