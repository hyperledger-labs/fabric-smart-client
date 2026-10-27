/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

// rowQuerier is satisfied by both *sql.DB and *sql.Tx.
type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// ReorderPrimaryKey changes table's primary key from (pkey, ns) to (ns, pkey)
// by rebuilding the table in one transaction. SQLite cannot replace a primary
// key in place. columnDefs declares the staging table's columns, without a
// primary key, such as "ns TEXT NOT NULL, pkey BYTEA NOT NULL"; the staging
// table gets PRIMARY KEY (ns, pkey). copyColumns is the column list copied, in
// the same order on both tables, such as "ns, pkey, val".
//
// The order is read once before the transaction, so a table that already has
// (ns, pkey) is left alone, and again after a write lock, so replicas sharing
// the file rebuild it once. Any other column order is an error and the table
// is left unchanged.
//
// The rebuild holds the write lock, so other writers wait for it. A replica
// that waits longer than busy_timeout fails with SQLITE_BUSY and finds the
// table already reordered on its next start. This function does not retry
// that error.
func ReorderPrimaryKey(readDB *sql.DB, writeDB common.WriteDB, table, columnDefs, copyColumns string) error {
	cols, err := PrimaryKeyColumns(readDB, table)
	if err != nil || cols == "ns,pkey" {
		return err
	}

	tx, err := writeDB.Begin()
	if err != nil {
		return errors.Wrap(err, "failed to initiate db transaction")
	}
	defer func() { _ = tx.Rollback() }()

	// A write takes the reserved lock before the order is read again, so a
	// replica that finishes the rebuild first is visible here. Do not remove
	// it: without it two replicas both read the order as readers, and the one
	// that upgrades to a writer second fails with SQLITE_BUSY.
	if _, err := tx.Exec("UPDATE " + table + " SET ns = ns WHERE 0"); err != nil { //nolint:gosec // table name comes from configuration, not user input
		return errors.Wrapf(err, "failed locking table [%s]", table)
	}
	if cols, err = PrimaryKeyColumns(tx, table); err != nil {
		return err
	}
	if cols == "ns,pkey" {
		return nil
	}
	if cols != "pkey,ns" {
		return errors.Errorf("table [%s] primary key is (%s), expected (ns, pkey)", table, cols)
	}

	logger.Infof("reordering primary key of table [%s] to (ns, pkey)", table)
	staging := table + "_fsc_nskey"
	statements := []string{
		"CREATE TABLE " + staging + " (" + columnDefs + ", PRIMARY KEY (ns, pkey))",
		"INSERT INTO " + staging + " (" + copyColumns + ") SELECT " + copyColumns + " FROM " + table,
		"DROP TABLE " + table,
		"ALTER TABLE " + staging + " RENAME TO " + table,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil { //nolint:gosec // table name and column list come from configuration, not user input
			return errors.Wrapf(err, "failed reordering primary key of table [%s]", table)
		}
	}
	return tx.Commit()
}

// PrimaryKeyColumns returns table's primary-key columns in index order, joined
// by commas. The table name is bound, so a connection that rejects
// double-quoted string literals still works.
func PrimaryKeyColumns(q rowQuerier, table string) (string, error) {
	var cols sql.NullString
	if err := q.QueryRow(`
		SELECT group_concat(ii.name, ',' ORDER BY ii.seqno)
		FROM pragma_index_list(?) il, pragma_index_info(il.name) ii
		WHERE il.origin = 'pk'`,
		table,
	).Scan(&cols); err != nil {
		return "", errors.Wrapf(err, "failed reading primary key of table [%s]", table)
	}
	if !cols.Valid || cols.String == "" {
		return "", errors.Errorf("table [%s] has no primary key", table)
	}
	return cols.String, nil
}
