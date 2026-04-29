/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"database/sql"
	errors2 "errors"
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

func NewVaultStore(dbs *common3.RWDB, tables common4.TableNames) (*VaultStore, error) {
	return newVaultStore(dbs.ReadDB, dbs.WriteDB, common4.VaultTables{
		StateTable:  tables.State,
		StatusTable: tables.Status,
	}), nil
}

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

	if len(txIDs) > 0 {
		query, params, err := db.SetStatusesBusy(txIDs)
		if err != nil {
			_ = tx.Rollback()
			return errors.Wrapf(err, "failed building busy query")
		}
		if err := execOrRollback(ctx, tx, query, params); err != nil {
			return errors.Wrapf(err, "failed setting tx to busy")
		}
	}
	if len(writes) > 0 || len(metaWrites) > 0 {
		query, params, err := db.UpsertStates(writes, metaWrites)
		if err != nil {
			_ = tx.Rollback()
			return errors.Wrapf(err, "failed building upsert states query")
		}
		if err := execOrRollback(ctx, tx, query, params); err != nil {
			return errors.Wrapf(err, "failed writing state")
		}
	}
	if len(txIDs) > 0 {
		query, params, err := db.SetStatusesValid(txIDs)
		if err != nil {
			_ = tx.Rollback()
			return errors.Wrapf(err, "failed building valid query")
		}
		if err := execOrRollback(ctx, tx, query, params); err != nil {
			return errors.Wrapf(err, "failed setting tx to valid")
		}
	}

	if err := tx.Commit(); err != nil {
		if err2 := tx.Rollback(); err2 != nil {
			return errors2.Join(err, err2)
		}
		return err
	}

	return nil
}

func execOrRollback(ctx context.Context, tx *sql.Tx, query string, params []any) error {
	logger.Debug(query, len(params), params)

	if _, err := tx.ExecContext(ctx, query, params...); err != nil {
		if err2 := tx.Rollback(); err2 != nil {
			return errors2.Join(err, err2)
		}
		return err
	}
	return nil
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
