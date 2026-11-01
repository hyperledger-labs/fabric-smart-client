/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// rowQuerier is satisfied by *sql.DB, *sql.Conn, *sql.Tx and migrate.Execer.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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
	ctx := context.Background()
	cols, err := readPrimaryKeyColumns(ctx, readDB, table)
	if err != nil || cols == "ns,pkey" {
		return err
	}

	tx, err := writeDB.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to initiate db transaction")
	}
	defer func() { _ = tx.Rollback() }()

	// A write takes the reserved lock before the order is read again, so a
	// replica that finishes the rebuild first is visible here. Do not remove
	// it: without it two replicas both read the order as readers, and the one
	// that upgrades to a writer second fails with SQLITE_BUSY.
	if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET ns = ns WHERE 0"); err != nil { //nolint:gosec // table name comes from configuration, not user input
		return errors.Wrapf(err, "failed locking table [%s]", table)
	}
	if err := ReorderPrimaryKeyTx(ctx, tx, table, columnDefs, copyColumns); err != nil {
		return err
	}
	return tx.Commit()
}

// ReorderPrimaryKeyTx is ReorderPrimaryKey inside the caller's transaction q,
// which must already hold the write lock, such as a BEGIN IMMEDIATE
// transaction: it reads the key order and rebuilds a table keyed (pkey, ns).
// Any order other than (ns, pkey) or (pkey, ns) is an error.
func ReorderPrimaryKeyTx(ctx context.Context, q migrate.Execer, table, columnDefs, copyColumns string) error {
	cols, err := readPrimaryKeyColumns(ctx, q, table)
	if err != nil || cols == "ns,pkey" {
		return err
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
		if _, err := q.ExecContext(ctx, statement); err != nil {
			return errors.Wrapf(err, "failed reordering primary key of table [%s]", table)
		}
	}
	return nil
}

// PrimaryKeyColumns returns table's primary-key columns in index order, joined
// by commas. The table name is bound, so a connection that rejects
// double-quoted string literals still works.
func PrimaryKeyColumns(q rowQuerier, table string) (string, error) {
	return readPrimaryKeyColumns(context.Background(), q, table)
}

func readPrimaryKeyColumns(ctx context.Context, q rowQuerier, table string) (string, error) {
	var cols sql.NullString
	if err := q.QueryRowContext(ctx, `
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
