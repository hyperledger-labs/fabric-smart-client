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
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

type VaultStore struct {
	*common.VaultStore

	tables  common.VaultTables
	readDB  *sql.DB
	writeDB sqlcommon.WriteDB
}

func NewVaultStore(dbs *drivercommon.RWDB, tables common.TableNames) (*VaultStore, error) {
	return buildVaultStore(dbs.ReadDB, sqlite.NewRetryWriteDB(dbs.WriteDB), common.VaultTables{
		StateTable:  tables.State,
		StatusTable: tables.Status,
	}, sqlcommon.QueryTimeout(dbs.QueryTimeout)), nil
}

func buildVaultStore(readDB *sql.DB, writeDB sqlcommon.WriteDB, tables common.VaultTables, timeout sqlcommon.QueryTimeout) *VaultStore {
	return &VaultStore{
		VaultStore: common.NewVaultStore(writeDB, readDB, tables, &sqlite.ErrorMapper{}, encodeKey, sqlite.IsolationLevels, timeout),
		tables:     tables,
		readDB:     readDB,
		writeDB:    writeDB,
	}
}

// encodeKey binds keys as TEXT. SQLite stores TEXT with its length, so U+0000
// survives, and its default BINARY collation compares keys bytewise.
func encodeKey(k driver.PKey) any { return k }

// Store marks txIDs busy, upserts writes and metaWrites, and marks txIDs valid,
// all in one database transaction, bounded as a whole by the store's query
// timeout. Database errors are mapped by sqlite.ErrorMapper.
func (db *VaultStore) Store(ctx context.Context, txIDs []driver.TxID, writes driver.Writes, metaWrites driver.MetaWrites) error {
	if len(txIDs) == 0 && len(writes) == 0 && len(metaWrites) == 0 {
		logger.DebugfContext(ctx, "Nothing to write")
		return nil
	}
	return (&sqlite.ErrorMapper{}).WrapError(db.storeTx(ctx, txIDs, writes, metaWrites))
}

func (db *VaultStore) storeTx(ctx context.Context, txIDs []driver.TxID, writes driver.Writes, metaWrites driver.MetaWrites) error {
	db.GlobalLock.RLock()
	defer db.GlobalLock.RUnlock()

	ctx, cancel := db.Timeout.Context(ctx)
	defer cancel()
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

// stateColumns declares the state table's columns without the primary key, for
// the staging table of a primary-key reorder.
const stateColumns = "ns TEXT NOT NULL, pkey TEXT NOT NULL, val BYTEA DEFAULT '', kversion BYTEA DEFAULT '', metadata BYTEA DEFAULT ''"

// CreateSchema creates the status and state tables if they do not exist. The
// state table's primary key is (ns, pkey). A table whose primary key is
// (pkey, ns) is rebuilt in one transaction, copying every row. The key order
// is read once before that transaction, so an upgraded table is left alone,
// and again after a write lock, so replicas sharing the file rebuild it once.
func (db *VaultStore) CreateSchema() error {
	if err := sqlcommon.InitSchema(db.writeDB, db.schema()); err != nil {
		return err
	}
	return sqlite.ReorderPrimaryKey(db.readDB, db.writeDB, db.tables.StateTable, stateColumns, "ns, pkey, val, kversion, metadata")
}

// StoreName returns the state table name; it identifies the vault in the
// versions table.
func (db *VaultStore) StoreName() string { return db.tables.StateTable }

// Steps returns the vault's schema steps. Step 1 creates the status and state
// tables if they do not exist and rebuilds a state table keyed (pkey, ns).
// Step 2, a required contract step, rewrites gob metadata as protobuf (see
// common.VaultStore.MetadataToProtobuf); binaries that read only gob refuse the
// vault afterwards, and this binary refuses it before.
func (db *VaultStore) Steps() []migrate.Step {
	return []migrate.Step{{
		Version: 1,
		Name:    "create tables",
		SQL:     []string{db.schema()},
		Go: func(ctx context.Context, q migrate.Execer) error {
			return sqlite.ReorderPrimaryKeyTx(ctx, q, db.tables.StateTable, stateColumns, "ns, pkey, val, kversion, metadata")
		},
	}, {
		Version:  2,
		Name:     "metadata as protobuf",
		Go:       db.MetadataToProtobuf,
		Contract: true,
		Required: true,
	}}
}

func (db *VaultStore) schema() string {
	return fmt.Sprintf(`
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
		db.tables.StateTable)
}
