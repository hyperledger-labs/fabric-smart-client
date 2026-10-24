/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"database/sql"
	"fmt"

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
	return buildVaultStore(dbs.ReadDB, dbs.WriteDB, common4.VaultTables{
		StateTable:  tables.State,
		StatusTable: tables.Status,
	}), nil
}

// buildVaultStore is the internal constructor.
func buildVaultStore(readDB, writeDB *sql.DB, tables common4.VaultTables) *VaultStore {
	return &VaultStore{
		VaultStore: common4.NewVaultStore(writeDB, readDB, tables, &postgres2.ErrorMapper{}, encodeKey, postgres2.IsolationLevels),
		tables:     tables,
		writeDB:    writeDB,
	}
}

// encodeKey binds keys as BYTEA. Postgres TEXT rejects U+0000, which every
// composite key contains, and orders by the database collation; BYTEA stores any
// key verbatim and compares bytewise, as Fabric orders keys.
func encodeKey(k driver.PKey) any { return []byte(k) }

func (db *VaultStore) Store(ctx context.Context, txIDs []driver.TxID, writes driver.Writes, metaWrites driver.MetaWrites) error {
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
		logger.Debug(query, len(params), params)
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrap(err, "failed setting tx to busy")
		}
	}
	if len(writes) > 0 || len(metaWrites) > 0 {
		query, params, err := db.UpsertStates(writes, metaWrites)
		if err != nil {
			return errors.Wrap(err, "failed building upsert states query")
		}
		logger.Debug(query, len(params), params)
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrap(err, "failed writing state")
		}
	}
	if len(txIDs) > 0 {
		query, params, err := db.SetStatusesValid(txIDs)
		if err != nil {
			return errors.Wrap(err, "failed building valid query")
		}
		logger.Debug(query, len(params), params)
		if _, err := tx.ExecContext(ctx, query, params...); err != nil {
			return errors.Wrap(err, "failed setting tx to valid")
		}
	}

	return tx.Commit()
}

// CreateSchema creates the status and state tables if they do not exist. The
// state table's primary key is (ns, pkey). An existing table is then brought to
// that layout: pkey is converted from TEXT to BYTEA when needed (see
// convertTextKeys), and a primary key of (pkey, ns) is reordered to (ns, pkey).
// Each step locks the table only when a change is required, and replicas
// sharing the database perform each change once.
func (db *VaultStore) CreateSchema() error {
	if err := common.InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		pos SERIAL PRIMARY KEY,
		tx_id TEXT UNIQUE NOT NULL,
		code INT NOT NULL DEFAULT (%d),
		message TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS tx_id_%s ON %s ( tx_id );

	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		val BYTEA,
		kversion BYTEA,
		metadata BYTEA,
		PRIMARY KEY (ns, pkey)
	);`, db.tables.StatusTable,
		driver.Unknown,
		db.tables.StatusTable, db.tables.StatusTable,
		db.tables.StateTable)); err != nil {
		return err
	}
	if err := db.convertTextKeys(); err != nil {
		return err
	}
	return db.reorderPrimaryKey()
}

// convertTextKeys converts a state table whose pkey column is TEXT to BYTEA in
// place. A TEXT pkey column holds keys in an escaped form, U+0000 as "?0?" and
// U+10FFFF as "?1?", which keys bound by encodeKey would never match. The
// conversion maps both sequences back to their UTF-8 bytes, so every key reads
// back as it did through the escaped form. A literal "?0?" or "?1?" in a key is
// indistinguishable from an escape and is converted too; where the two overlap,
// as in "?0?1?", "?0?" is converted first.
//
// The conversion rewrites the table under an ACCESS EXCLUSIVE lock. The column
// type is read once without the lock, so opening a BYTEA table never blocks,
// and again under it, so replicas sharing the database convert the table once.
// Any pkey type other than TEXT or BYTEA is an error.
func (db *VaultStore) convertTextKeys() error {
	pkeyType, err := db.pkeyType(db.writeDB)
	if err != nil || pkeyType == "bytea" {
		return err
	}

	tx, err := db.writeDB.Begin()
	if err != nil {
		return errors.Wrap(err, "failed to initiate db transaction")
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("LOCK TABLE " + db.tables.StateTable + " IN ACCESS EXCLUSIVE MODE"); err != nil {
		return errors.Wrapf(err, "failed locking state table [%s]", db.tables.StateTable)
	}
	if pkeyType, err = db.pkeyType(tx); err != nil {
		return err
	}
	switch pkeyType {
	case "bytea":
		return nil
	case "text":
		logger.Infof("converting pkey of state table [%s] from text to bytea", db.tables.StateTable)
		// The replacements run on the hex form, since Postgres has no bytea
		// replace. A match cannot straddle a byte boundary: that would need the
		// byte 0xF3 followed by 0x03 or 0x13, which is not valid UTF-8.
		if _, err := tx.Exec("ALTER TABLE " + db.tables.StateTable + ` ALTER COLUMN pkey TYPE BYTEA USING
			decode(replace(replace(encode(convert_to(pkey, 'UTF8'), 'hex'), '3f303f', '00'), '3f313f', 'f48fbfbf'), 'hex')`); err != nil {
			return errors.Wrapf(err, "failed converting pkey of state table [%s] to bytea", db.tables.StateTable)
		}
		return tx.Commit()
	default:
		return errors.Errorf("state table [%s] stores pkey as [%s], expected [bytea]", db.tables.StateTable, pkeyType)
	}
}

// rowQuerier is satisfied by both *sql.DB and *sql.Tx.
type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// pkeyType returns the SQL type of the state table's pkey column.
func (db *VaultStore) pkeyType(q rowQuerier) (string, error) {
	var t string
	if err := q.QueryRow(
		"SELECT format_type(atttypid, atttypmod) FROM pg_attribute WHERE attrelid = $1::regclass AND attname = 'pkey'",
		db.tables.StateTable,
	).Scan(&t); err != nil {
		return "", errors.Wrapf(err, "failed reading pkey type of state table [%s]", db.tables.StateTable)
	}
	return t, nil
}

// reorderPrimaryKey changes a primary key of (pkey, ns) to (ns, pkey). Every
// state query filters on one namespace, and (ns, pkey) lets that scan start at
// the namespace. The key order is read once without a lock, so an upgraded
// table never blocks, and again under ACCESS EXCLUSIVE, so replicas sharing
// the database reorder the table once. The constraint name comes from
// pg_constraint, because the table name is configurable. Any other column
// order is an error and the table is left unchanged.
func (db *VaultStore) reorderPrimaryKey() error {
	cols, err := db.primaryKeyColumns(db.writeDB)
	if err != nil || cols == "ns,pkey" {
		return err
	}

	tx, err := db.writeDB.Begin()
	if err != nil {
		return errors.Wrap(err, "failed to initiate db transaction")
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("LOCK TABLE " + db.tables.StateTable + " IN ACCESS EXCLUSIVE MODE"); err != nil {
		return errors.Wrapf(err, "failed locking state table [%s]", db.tables.StateTable)
	}
	if cols, err = db.primaryKeyColumns(tx); err != nil {
		return err
	}
	if cols == "ns,pkey" {
		return nil
	}
	if cols != "pkey,ns" {
		return errors.Errorf("state table [%s] primary key is (%s), expected (ns, pkey)", db.tables.StateTable, cols)
	}
	constraint, err := db.primaryKeyConstraint(tx)
	if err != nil {
		return err
	}
	logger.Infof("reordering primary key of state table [%s] to (ns, pkey)", db.tables.StateTable)
	if _, err := tx.Exec("ALTER TABLE " + db.tables.StateTable + " DROP CONSTRAINT " + constraint); err != nil {
		return errors.Wrapf(err, "failed dropping primary key of state table [%s]", db.tables.StateTable)
	}
	if _, err := tx.Exec("ALTER TABLE " + db.tables.StateTable + " ADD PRIMARY KEY (ns, pkey)"); err != nil {
		return errors.Wrapf(err, "failed adding primary key (ns, pkey) on state table [%s]", db.tables.StateTable)
	}
	return tx.Commit()
}

// primaryKeyColumns returns the state table's primary-key columns in index
// order, joined by commas.
func (db *VaultStore) primaryKeyColumns(q rowQuerier) (string, error) {
	var cols sql.NullString
	if err := q.QueryRow(`
		SELECT string_agg(a.attname, ',' ORDER BY cols.ord)
		FROM pg_index i
		CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS cols(attnum, ord)
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = cols.attnum
		WHERE i.indrelid = $1::regclass AND i.indisprimary`,
		db.tables.StateTable,
	).Scan(&cols); err != nil {
		return "", errors.Wrapf(err, "failed reading primary key of state table [%s]", db.tables.StateTable)
	}
	if !cols.Valid || cols.String == "" {
		return "", errors.Errorf("state table [%s] has no primary key", db.tables.StateTable)
	}
	return cols.String, nil
}

// primaryKeyConstraint returns the state table's primary-key constraint name,
// quoted for use in DDL.
func (db *VaultStore) primaryKeyConstraint(q rowQuerier) (string, error) {
	var name string
	if err := q.QueryRow(
		"SELECT quote_ident(conname) FROM pg_constraint WHERE conrelid = $1::regclass AND contype = 'p'",
		db.tables.StateTable,
	).Scan(&name); err != nil {
		return "", errors.Wrapf(err, "failed reading primary key constraint of state table [%s]", db.tables.StateTable)
	}
	return name, nil
}
