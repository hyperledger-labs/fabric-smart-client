/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/pkg/errors"
)

type VaultPersistence struct {
	*common.VaultPersistence

	tables  common.VaultTables
	writeDB *sql.DB

	ci common.Interpreter
}

func NewVaultPersistence(opts common.Opts, tablePrefix string) (*VaultPersistence, error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newTxCodePersistence(readDB, writeDB, common.VaultTables{
		StateTable:  fmt.Sprintf("%s_state", tablePrefix),
		StatusTable: fmt.Sprintf("%s_status", tablePrefix),
	}), nil
}

func newTxCodePersistence(readDB, writeDB *sql.DB, tables common.VaultTables) *VaultPersistence {
	ci := NewInterpreter()
	return &VaultPersistence{
		VaultPersistence: common.NewVaultPersistence(readDB, writeDB, tables, &errorMapper{}, ci, newSanitizer()),
		tables:           tables,
		writeDB:          writeDB,
		ci:               ci,
	}
}

func (db *VaultPersistence) Store(txIDs []driver.TxID, writes driver.Writes, metaWrites driver.MetaWrites) error {
	if len(txIDs) == 0 && len(writes) == 0 && len(metaWrites) == 0 {
		logger.Debugf("Nothing to write")
		return nil
	}

	if err := db.AcquireWLocks(txIDs...); err != nil {
		return err
	}
	defer db.ReleaseWLocks(txIDs...)

	params := make([]any, 0)
	queryBuilder := strings.Builder{}
	queryBuilder.WriteString(`
		BEGIN;
	`)

	if len(txIDs) > 0 {
		q, ps := db.SetStatusesBusy(txIDs, len(params)+1)
		params = append(params, ps...)
		queryBuilder.WriteString(q)
	}

	if len(writes) > 0 || len(metaWrites) > 0 {
		q, ps, err := db.UpsertStates(writes, metaWrites, len(params)+1)
		if err != nil {
			return err
		}
		params = append(params, ps...)
		queryBuilder.WriteString(q)
	}

	if len(txIDs) > 0 {
		q, ps := db.UpdateStatusesValid(txIDs, len(params)+1)
		params = append(params, ps...)
		queryBuilder.WriteString(q)
	}

	queryBuilder.WriteString(`
		COMMIT;
	`)

	query := queryBuilder.String()

	logger.Info(query, params)
	if _, err := db.writeDB.Exec(query, params...); err != nil {
		return errors.Wrapf(err, "failed to store writes and metawrites for %d txs", len(txIDs))
	}
	return nil
}

func (db *VaultPersistence) CreateSchema() error {
	return common.InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		pos INTEGER PRIMARY KEY,
		tx_id TEXT UNIQUE NOT NULL,
		code INT NOT NULL DEFAULT (%d),
		message TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS tx_id_%s ON %s ( tx_id );

	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA DEFAULT '',
		kversion BYTEA DEFAULT '',
		metadata BYTEA DEFAULT '',
		PRIMARY KEY (pkey, ns)
	);`, db.tables.StatusTable,
		driver.Unknown,
		db.tables.StatusTable, db.tables.StatusTable,
		db.tables.StateTable))
}
