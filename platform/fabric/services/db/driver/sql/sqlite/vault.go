/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

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
	readDB  *sql.DB
	writeDB common5.WriteDB
}

func NewVaultStore(dbs *common3.RWDB, tables common.TableNames) (*VaultStore, error) {
	return buildVaultStore(dbs.ReadDB, sqlite2.NewRetryWriteDB(dbs.WriteDB), common.VaultTables{
		StateTable:  tables.State,
		StatusTable: tables.Status,
	}), nil
}

func buildVaultStore(readDB *sql.DB, writeDB common5.WriteDB, tables common.VaultTables) *VaultStore {
	return &VaultStore{
		VaultStore: common.NewVaultStore(writeDB, readDB, tables, &sqlite2.ErrorMapper{}, encodeKey, sqlite2.IsolationLevels),
		tables:     tables,
		readDB:     readDB,
		writeDB:    writeDB,
	}
}

// encodeKey binds keys as TEXT. SQLite stores TEXT with its length, so U+0000
// survives, and its default BINARY collation compares keys bytewise.
func encodeKey(k driver.PKey) any { return k }

func (db *VaultStore) Store(ctx context.Context, txIDs []driver.TxID, writes driver.Writes, metaWrites driver.MetaWrites) error {
	if len(txIDs) == 0 && len(writes) == 0 && len(metaWrites) == 0 {
		logger.DebugfContext(ctx, "Nothing to write")
		return nil
	}

	db.GlobalLock.RLock()
	defer db.GlobalLock.RUnlock()

	tx, err := db.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to initiate db transaction")
	}
	defer func() { _ = tx.Rollback() }()

	if len(txIDs) > 0 {
		query, params, err := db.SetStatusesBusy(txIDs)
		if err != nil {
			return errors.Wrap(err, "failed building busy query")
		}
		logger.Debug(query, txIDs)
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrap(err, "failed setting tx to busy")
		}
	}

	if len(writes) > 0 || len(metaWrites) > 0 {
		query, params, err := db.UpsertStates(writes, metaWrites)
		if err != nil {
			return errors.Wrap(err, "failed building upsert states query")
		}
		logger.Debug(query, logging.Keys(writes))
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrap(err, "failed writing state")
		}
	}

	if len(txIDs) > 0 {
		query, params, err := db.SetStatusesValid(txIDs)
		if err != nil {
			return errors.Wrap(err, "failed building valid query")
		}
		logger.Debug(query, txIDs)
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrap(err, "failed setting tx to valid")
		}
	}

	return tx.Commit()
}

// CreateSchema creates the status and state tables if they do not exist. The
// state table's primary key is (ns, pkey). A table whose primary key is
// (pkey, ns) is rebuilt in one transaction, copying every row. The key order
// is read once before that transaction, so an upgraded table is left alone,
// and again after a write lock, so replicas sharing the file rebuild it once.
func (db *VaultStore) CreateSchema() error {
	if err := common5.InitSchema(db.writeDB, fmt.Sprintf(`
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
		PRIMARY KEY (ns, pkey)
	);`, db.tables.StatusTable,
		driver.Unknown,
		db.tables.StatusTable, db.tables.StatusTable,
		db.tables.StateTable)); err != nil {
		return err
	}
	return db.reorderPrimaryKey()
}

// rowQuerier is satisfied by both *sql.DB and *sql.Tx.
type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// reorderPrimaryKey changes a primary key of (pkey, ns) to (ns, pkey) by
// rebuilding the table in one transaction, copying every row. SQLite cannot
// replace a primary key in place. Any other column order is an error and the
// table is left unchanged.
//
// The rebuild holds the write lock, so other writers wait for it. A replica
// that waits longer than busy_timeout fails with SQLITE_BUSY and finds the
// table already reordered on its next start.
func (db *VaultStore) reorderPrimaryKey() error {
	cols, err := db.primaryKeyColumns(db.readDB)
	if err != nil || cols == "ns,pkey" {
		return err
	}

	tx, err := db.writeDB.Begin()
	if err != nil {
		return errors.Wrap(err, "failed to initiate db transaction")
	}
	defer func() { _ = tx.Rollback() }()

	// A write takes the reserved lock before the order is read again, so a
	// replica that finishes the rebuild first is visible here. Do not remove
	// it: without it two replicas both read the order as readers, and the one
	// that upgrades to a writer second fails with SQLITE_BUSY.
	table := db.tables.StateTable
	if _, err := tx.Exec("UPDATE " + table + " SET ns = ns WHERE 0"); err != nil { //nolint:gosec // table name comes from configuration, not user input
		return errors.Wrapf(err, "failed locking state table [%s]", table)
	}
	if cols, err = db.primaryKeyColumns(tx); err != nil {
		return err
	}
	if cols == "ns,pkey" {
		return nil
	}
	if cols != "pkey,ns" {
		return errors.Errorf("state table [%s] primary key is (%s), expected (ns, pkey)", table, cols)
	}

	logger.Infof("reordering primary key of state table [%s] to (ns, pkey)", table)
	staging := table + "_fsc_nskey"
	statements := []string{
		`CREATE TABLE ` + staging + ` (
			ns TEXT NOT NULL,
			pkey TEXT NOT NULL,
			val BYTEA DEFAULT '',
			kversion BYTEA DEFAULT '',
			metadata BYTEA DEFAULT '',
			PRIMARY KEY (ns, pkey)
		)`,
		`INSERT INTO ` + staging + ` (ns, pkey, val, kversion, metadata)
			SELECT ns, pkey, val, kversion, metadata FROM ` + table,
		`DROP TABLE ` + table,
		`ALTER TABLE ` + staging + ` RENAME TO ` + table,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return errors.Wrapf(err, "failed reordering primary key of state table [%s]", table)
		}
	}
	return tx.Commit()
}

// primaryKeyColumns returns the state table's primary-key columns in index
// order, joined by commas.
func (db *VaultStore) primaryKeyColumns(q rowQuerier) (string, error) {
	var cols sql.NullString
	if err := q.QueryRow(`
		SELECT group_concat(ii.name, ',' ORDER BY ii.seqno)
		FROM pragma_index_list(?) il, pragma_index_info(il.name) ii
		WHERE il.origin = 'pk'`,
		db.tables.StateTable,
	).Scan(&cols); err != nil {
		return "", errors.Wrapf(err, "failed reading primary key of state table [%s]", db.tables.StateTable)
	}
	if !cols.Valid || cols.String == "" {
		return "", errors.Errorf("state table [%s] has no primary key", db.tables.StateTable)
	}
	return cols.String, nil
}
