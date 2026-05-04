/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common5 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

type VaultStore struct {
	*common.VaultStore

	tables  common.VaultTables
	writeDB common5.WriteDB
}

func NewVaultStore(dbs *common3.RWDB, tables common.TableNames) (*VaultStore, error) {
	return newVaultStore(dbs.ReadDB, sqlite2.NewRetryWriteDB(dbs.WriteDB), common.VaultTables{
		StateTable:  tables.State,
		StatusTable: tables.Status,
	}), nil
}

func newVaultStore(readDB *sql.DB, writeDB common5.WriteDB, tables common.VaultTables) *VaultStore {
	return &VaultStore{
		VaultStore: common.NewVaultStore(writeDB, readDB, tables, &sqlite2.ErrorMapper{}, sq.Question, sqlite2.NewSanitizer(), sqlite2.IsolationLevels),
		tables:     tables,
		writeDB:    writeDB,
	}
}

func (db *VaultStore) Store(ctx context.Context, txIDs []driver.TxID, writes driver.Writes, metaWrites driver.MetaWrites) error {
	if len(txIDs) == 0 && len(writes) == 0 && len(metaWrites) == 0 {
		logger.DebugfContext(ctx, "Nothing to write")
		return nil
	}

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
		logger.Debug(query, txIDs)
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrapf(err, "failed setting tx to busy")
		}
	}

	if len(writes) > 0 || len(metaWrites) > 0 {
		query, params, err := db.UpsertStates(writes, metaWrites)
		if err != nil {
			return errors.Wrapf(err, "failed building upsert states query")
		}
		logger.Debug(query, logging.Keys(writes))
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrapf(err, "failed writing state")
		}
	}

	if len(txIDs) > 0 {
		query, params, err := db.SetStatusesValid(txIDs)
		if err != nil {
			return errors.Wrapf(err, "failed building valid query")
		}
		logger.Debug(query, txIDs)
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrapf(err, "failed setting tx to valid")
		}
	}

	return tx.Commit()
}

func (db *VaultStore) CreateSchema() error {
	return common5.InitSchema(db.writeDB, fmt.Sprintf(`
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
