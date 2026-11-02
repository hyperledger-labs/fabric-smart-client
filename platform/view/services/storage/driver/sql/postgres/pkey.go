/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// rowQuerier is satisfied by *sql.DB, *sql.Conn, *sql.Tx and migrate.Execer.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ReorderPrimaryKey changes table's primary key from (pkey, ns) to (ns, pkey).
// Every query that uses this key filters on one namespace, and (ns, pkey) lets
// that scan start at the namespace. The order is read once without a lock, so a
// table that already has (ns, pkey) never blocks, and again under ACCESS
// EXCLUSIVE, so replicas sharing the database reorder the table once. The
// constraint name comes from pg_constraint, because the table name is
// configurable. Any other column order is an error and the table is left
// unchanged.
func ReorderPrimaryKey(writeDB *sql.DB, table string) error {
	ctx := context.Background()
	_, cols, err := readPrimaryKey(ctx, writeDB, table)
	if err != nil || cols == "ns,pkey" {
		return err
	}

	tx, err := writeDB.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to initiate db transaction")
	}
	defer func() { _ = tx.Rollback() }()
	if err := ReorderPrimaryKeyTx(ctx, tx, table); err != nil {
		return err
	}
	return tx.Commit()
}

// ReorderPrimaryKeyTx is ReorderPrimaryKey inside the caller's transaction q.
// It reads the key order without a lock, so a table keyed (ns, pkey) is never
// locked; otherwise it locks table in ACCESS EXCLUSIVE mode until q ends, reads
// the order again and replaces a (pkey, ns) key, so a node running
// ReorderPrimaryKey on the same table at the same time waits and then finds the
// key reordered. Any order other than (ns, pkey) or (pkey, ns) is an error.
func ReorderPrimaryKeyTx(ctx context.Context, q migrate.Execer, table string) error {
	if _, cols, err := readPrimaryKey(ctx, q, table); err != nil || cols == "ns,pkey" {
		return err
	}
	if _, err := q.ExecContext(ctx, "LOCK TABLE "+table+" IN ACCESS EXCLUSIVE MODE"); err != nil {
		return errors.Wrapf(err, "failed locking table [%s]", table)
	}
	constraint, cols, err := readPrimaryKey(ctx, q, table)
	if err != nil || cols == "ns,pkey" {
		return err
	}
	if cols != "pkey,ns" {
		return errors.Errorf("table [%s] primary key is (%s), expected (ns, pkey)", table, cols)
	}
	logger.Infof("reordering primary key of table [%s] to (ns, pkey)", table)
	if _, err := q.ExecContext(ctx, "ALTER TABLE "+table+" DROP CONSTRAINT "+constraint); err != nil {
		return errors.Wrapf(err, "failed dropping primary key of table [%s]", table)
	}
	if _, err := q.ExecContext(ctx, "ALTER TABLE "+table+" ADD PRIMARY KEY (ns, pkey)"); err != nil {
		return errors.Wrapf(err, "failed adding primary key (ns, pkey) on table [%s]", table)
	}
	return nil
}

// PrimaryKey returns table's primary-key constraint name, quoted for use in
// DDL, and its columns in key order, joined by commas.
func PrimaryKey(q rowQuerier, table string) (constraint, cols string, err error) {
	return readPrimaryKey(context.Background(), q, table)
}

func readPrimaryKey(ctx context.Context, q rowQuerier, table string) (constraint, cols string, err error) {
	if err := q.QueryRowContext(ctx, `
		SELECT quote_ident(c.conname), string_agg(a.attname, ',' ORDER BY k.ord)
		FROM pg_constraint c
		CROSS JOIN LATERAL unnest(c.conkey) WITH ORDINALITY AS k(attnum, ord)
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
		WHERE c.conrelid = $1::regclass AND c.contype = 'p'
		GROUP BY c.conname`,
		table,
	).Scan(&constraint, &cols); errors.Is(err, sql.ErrNoRows) {
		return "", "", errors.Errorf("table [%s] has no primary key", table)
	} else if err != nil {
		return "", "", errors.Wrapf(err, "failed reading primary key of table [%s]", table)
	}
	return constraint, cols, nil
}
