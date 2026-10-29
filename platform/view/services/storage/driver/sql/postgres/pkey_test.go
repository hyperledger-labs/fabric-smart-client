/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openReorderDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", setupDB(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func createKeyedTable(t *testing.T, db *sql.DB, primaryKey string) {
	t.Helper()
	_, err := db.Exec(`CREATE TABLE t (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		` + primaryKey + `
	)`)
	require.NoError(t, err)
}

// ReorderPrimaryKey changes a BYTEA table whose primary key is (pkey, ns).
// The constraint name is not the default, rows stay readable, and a second
// call does nothing.
func TestPostgresReorderPrimaryKey(t *testing.T) {
	t.Parallel()

	db := openReorderDB(t)
	createKeyedTable(t, db, "CONSTRAINT kvs_old_pk PRIMARY KEY (pkey, ns)")
	_, err := db.Exec("INSERT INTO t (ns, pkey, val) VALUES ($1, $2, $3), ($4, $5, $6)",
		"ns-a", []byte("k1"), []byte("a1"),
		"ns-b", []byte("k1"), []byte("b1"),
	)
	require.NoError(t, err)

	require.NoError(t, ReorderPrimaryKey(db, "t"))
	_, cols, err := PrimaryKey(db, "t")
	require.NoError(t, err)
	require.Equal(t, "ns,pkey", cols)
	var left int
	require.NoError(t, db.QueryRow("SELECT count(*) FROM pg_constraint WHERE conname = 'kvs_old_pk'").Scan(&left))
	require.Zero(t, left)
	require.NoError(t, ReorderPrimaryKey(db, "t"))

	var val []byte
	require.NoError(t, db.QueryRow("SELECT val FROM t WHERE ns = $1 AND pkey = $2", "ns-b", []byte("k1")).Scan(&val))
	require.Equal(t, []byte("b1"), val)
}

// A primary key other than (pkey, ns) or (ns, pkey) is not rewritten.
func TestPostgresReorderRejectsOtherPrimaryKey(t *testing.T) {
	t.Parallel()

	db := openReorderDB(t)
	createKeyedTable(t, db, "PRIMARY KEY (pkey)")
	_, err := db.Exec("INSERT INTO t (ns, pkey, val) VALUES ($1, $2, $3)", "ns", []byte("k1"), []byte("v"))
	require.NoError(t, err)

	err = ReorderPrimaryKey(db, "t")
	require.ErrorContains(t, err, "primary key is (pkey), expected (ns, pkey)")
	_, cols, err := PrimaryKey(db, "t")
	require.NoError(t, err)
	require.Equal(t, "pkey", cols)
	var n int
	require.NoError(t, db.QueryRow("SELECT count(*) FROM t").Scan(&n))
	require.Equal(t, 1, n)
}

// A table without a primary key is reported as such and left unchanged.
func TestPostgresReorderRejectsMissingPrimaryKey(t *testing.T) {
	t.Parallel()

	db := openReorderDB(t)
	_, err := db.Exec(`CREATE TABLE t (ns TEXT NOT NULL, pkey BYTEA NOT NULL, val BYTEA NOT NULL DEFAULT '')`)
	require.NoError(t, err)

	require.ErrorContains(t, ReorderPrimaryKey(db, "t"), "has no primary key")
	_, _, err = PrimaryKey(db, "t")
	require.ErrorContains(t, err, "has no primary key")
}

// Replicas sharing a table keyed (pkey, ns) all finish: the first to get the
// lock reorders it, and the others find (ns, pkey) under the lock. The test
// holds a lock on the table until every replica has read (pkey, ns) and is
// queued for the reorder lock. The old constraint name differs from the one
// ADD PRIMARY KEY gives the new key, so a replica that acts on what it read
// before the lock fails.
func TestPostgresReorderPrimaryKeyConcurrently(t *testing.T) {
	t.Parallel()

	db := openReorderDB(t)
	ctx := t.Context()
	createKeyedTable(t, db, "CONSTRAINT t_old_pk PRIMARY KEY (pkey, ns)")
	_, err := db.Exec("INSERT INTO t (ns, pkey, val) VALUES ($1, $2, $3)", "ns", []byte("k1"), []byte("v"))
	require.NoError(t, err)

	holder, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = holder.ExecContext(ctx, "LOCK TABLE t IN ACCESS SHARE MODE")
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() { errs[i] = ReorderPrimaryKey(db, "t") })
	}
	require.Eventually(t, func() bool {
		var waiting int
		err := db.QueryRowContext(ctx, "SELECT count(*) FROM pg_locks WHERE relation = 't'::regclass AND NOT granted").Scan(&waiting)
		return err == nil && waiting == len(errs)
	}, 30*time.Second, 10*time.Millisecond, "every replica queues for the reorder lock")
	require.NoError(t, holder.Rollback())
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	_, cols, err := PrimaryKey(db, "t")
	require.NoError(t, err)
	require.Equal(t, "ns,pkey", cols)
	var val []byte
	require.NoError(t, db.QueryRow("SELECT val FROM t WHERE ns = $1 AND pkey = $2", "ns", []byte("k1")).Scan(&val))
	require.Equal(t, []byte("v"), val)
}
