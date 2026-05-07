/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	common4 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

type VaultStore struct {
	*common4.VaultStore

	tables  common4.VaultTables
	writeDB *sql.DB
}

// NewVaultStore creates a postgres-backed VaultStore for the given
// read/write database and table names.
func NewVaultStore(dbs *common3.RWDB, tables common4.TableNames) (*VaultStore, error) {
	return newVaultStore(dbs.ReadDB, dbs.WriteDB, common4.VaultTables{
		StateTable:  tables.State,
		StatusTable: tables.Status,
	}), nil
}

// newVaultStore is the internal constructor. It uses sq.Dollar as the
// squirrel placeholder format for all PostgreSQL queries.
func newVaultStore(readDB, writeDB *sql.DB, tables common4.VaultTables) *VaultStore {
	return &VaultStore{
		VaultStore: common4.NewVaultStore(writeDB, readDB, tables, &postgres2.ErrorMapper{}, sq.Dollar, postgres2.NewSanitizer(), postgres2.IsolationLevels),
		tables:     tables,
		writeDB:    writeDB,
	}
}

func (db *VaultStore) Store(ctx context.Context, txIDs []driver.TxID, writes driver.Writes, metaWrites driver.MetaWrites) error {
	db.GlobalLock.RLock()
	defer db.GlobalLock.RUnlock()

	tx, err := db.writeDB.Begin()
	if err != nil {
		return errors.Wrapf(err, "failed to initiate db transaction")
	}
	defer func() { _ = tx.Rollback() }()

	if len(txIDs) > 0 {
		query, params, err := db.SetStatusesBusy(txIDs)
		if err != nil {
			return errors.Wrapf(err, "failed building busy query")
		}
		logger.Debug(query, len(params), params)
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrapf(err, "failed setting tx to busy")
		}
	}
	if len(writes) > 0 || len(metaWrites) > 0 {
		query, params, err := db.UpsertStates(writes, metaWrites)
		if err != nil {
			return errors.Wrapf(err, "failed building upsert states query")
		}
		logger.Debug(query, len(params), params)
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrapf(err, "failed writing state")
		}
	}
	if len(txIDs) > 0 {
		query, params, err := db.SetStatusesValid(txIDs)
		if err != nil {
			return errors.Wrapf(err, "failed building valid query")
		}
		logger.Debug(query, len(params), params)
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrapf(err, "failed setting tx to valid")
		}
	}

	return tx.Commit()
}

func (db *VaultStore) CreateSchema() error {
	return common.InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		pos SERIAL PRIMARY KEY,
		tx_id TEXT UNIQUE NOT NULL,
		code INT NOT NULL DEFAULT (%d),
		message TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS tx_id_%s ON %s ( tx_id );

	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA,
		kversion BYTEA,
		metadata BYTEA,
		PRIMARY KEY (pkey, ns)
	);`, db.tables.StatusTable,
		driver.Unknown,
		db.tables.StatusTable, db.tables.StatusTable,
		db.tables.StateTable))
}
