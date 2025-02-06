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

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type VaultPersistence struct {
	*common.VaultPersistence

	tables  common.VaultTables
	writeDB *sql.DB
	ci      common.Interpreter
}

func NewVaultPersistence(opts common.Opts, tablePrefix string) (*VaultPersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newVaultPersistence(readWriteDB, common.VaultTables{
		StateTable:  fmt.Sprintf("%s_state", tablePrefix),
		StatusTable: fmt.Sprintf("%s_status", tablePrefix),
	}), nil
}

func newVaultPersistence(readWriteDB *sql.DB, tables common.VaultTables) *VaultPersistence {
	ci := NewInterpreter()
	return &VaultPersistence{
		VaultPersistence: common.NewVaultPersistence(readWriteDB, readWriteDB, tables, &errorMapper{}, ci, newSanitizer(), isolationLevels),
		tables:           tables,
		writeDB:          readWriteDB,
		ci:               ci,
	}
}

func (db *VaultPersistence) Store(ctx context.Context, txIDs []driver.TxID, writes driver.Writes, metaWrites driver.MetaWrites) error {
	db.GlobalLock.RLock()
	defer db.GlobalLock.RUnlock()

	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start store")
	defer span.AddEvent("End store")

	tx, err := db.writeDB.Begin()
	if err != nil {
		return errors.Wrapf(err, "failed to initiate db transaction")
	}

	if len(txIDs) > 0 {
		query, params := db.VaultPersistence.SetStatusesBusy(txIDs, 1)
		if err := execOrRollback(tx, query, params); err != nil {
			return errors.Wrapf(err, "failed setting tx to busy")
		}
	}
	if len(writes) > 0 || len(metaWrites) > 0 {
		query, params, err := db.VaultPersistence.UpsertStates(writes, metaWrites, 1)
		if err != nil {
			return err
		}
		if err := execOrRollback(tx, query, params); err != nil {
			return errors.Wrapf(err, "failed writing state")
		}
	}
	if len(txIDs) > 0 {
		query, params := db.VaultPersistence.SetStatusesValid(txIDs, 1)
		if err := execOrRollback(tx, query, params); err != nil {
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

func execOrRollback(tx *sql.Tx, query string, params []any) error {
	logger.Debug(query, len(params), params)

	if _, err := tx.Exec(query, params...); err != nil {
		if err2 := tx.Rollback(); err2 != nil {
			return errors2.Join(err, err2)
		}
		return err
	}
	return nil
}

func (db *VaultPersistence) CreateSchema() error {
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
