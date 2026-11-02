/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"database/sql"

	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

func NewKeyValueStore(dbs *drivercommon.RWDB, tables sqlcommon.TableNames) (*KeyValueStore, error) {
	return buildKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS), nil
}

// KeyValueStore is the Postgres key-value store. Its schema reorders a primary
// key of (pkey, ns) to (ns, pkey) after the table exists.
type KeyValueStore struct {
	*sqlcommon.KeyValueStore

	writeDB *sql.DB
	table   string
}

func (db *KeyValueStore) CreateSchema() error {
	if err := db.KeyValueStore.CreateSchema(); err != nil {
		return err
	}
	return ReorderPrimaryKey(db.writeDB, db.table)
}

// Steps returns the store's schema steps. Step 1 creates the table if it does
// not exist and reorders a primary key of (pkey, ns) to (ns, pkey).
func (db *KeyValueStore) Steps() []migrate.Step {
	steps := db.KeyValueStore.Steps()
	steps[0].Go = func(ctx context.Context, q migrate.Execer) error {
		return ReorderPrimaryKeyTx(ctx, q, db.table)
	}
	return steps
}

func buildKeyValueStore(readDB, writeDB *sql.DB, table string) *KeyValueStore {
	errorWrapper := &ErrorMapper{}

	return &KeyValueStore{
		KeyValueStore: sqlcommon.NewKeyValueStore(writeDB, readDB, table, errorWrapper),
		writeDB:       writeDB,
		table:         table,
	}
}
