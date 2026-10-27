/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openReorderDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	return db
}

func textStaging(staging string) string {
	return `CREATE TABLE ` + staging + ` (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		PRIMARY KEY (ns, pkey)
	)`
}

func createKeyedTable(t *testing.T, db *sql.DB, primaryKey string) {
	t.Helper()
	_, err := db.Exec(`CREATE TABLE t (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		` + primaryKey + `
	)`)
	require.NoError(t, err)
}

// ReorderPrimaryKey rebuilds a table whose primary key is (pkey, ns). The
// connection rejects double-quoted string literals, so the catalog lookup must
// bind the table name. A second call does nothing.
func TestReorderPrimaryKey(t *testing.T) {
	t.Parallel()

	db := openReorderDB(t, "file:"+filepath.Join(t.TempDir(), "t.db")+"?_dqs=0")
	createKeyedTable(t, db, "PRIMARY KEY (pkey, ns)")
	_, err := db.Exec("INSERT INTO t (ns, pkey, val) VALUES ('ns-a', 'k1', 'a1'), ('ns-b', 'k1', 'b1')")
	require.NoError(t, err)

	require.NoError(t, ReorderPrimaryKey(db, db, "t", "ns, pkey, val", textStaging))
	cols, err := PrimaryKeyColumns(db, "t")
	require.NoError(t, err)
	require.Equal(t, "ns,pkey", cols)
	require.NoError(t, ReorderPrimaryKey(db, db, "t", "ns, pkey, val", textStaging))

	var val string
	require.NoError(t, db.QueryRow("SELECT val FROM t WHERE ns = 'ns-b' AND pkey = 'k1'").Scan(&val))
	require.Equal(t, "b1", val)
}

// A primary key other than (pkey, ns) or (ns, pkey) is not rewritten.
func TestReorderRejectsOtherPrimaryKey(t *testing.T) {
	t.Parallel()

	db := openReorderDB(t, "file:"+filepath.Join(t.TempDir(), "t.db"))
	createKeyedTable(t, db, "PRIMARY KEY (pkey)")
	_, err := db.Exec("INSERT INTO t (ns, pkey, val) VALUES ('ns', 'k1', 'v')")
	require.NoError(t, err)

	err = ReorderPrimaryKey(db, db, "t", "ns, pkey, val", textStaging)
	require.ErrorContains(t, err, "primary key is (pkey), expected (ns, pkey)")
	cols, err := PrimaryKeyColumns(db, "t")
	require.NoError(t, err)
	require.Equal(t, "pkey", cols)
	var n int
	require.NoError(t, db.QueryRow("SELECT count(*) FROM t").Scan(&n))
	require.Equal(t, 1, n)
}

// A table without a primary key is reported as such.
func TestReorderRejectsMissingPrimaryKey(t *testing.T) {
	t.Parallel()

	db := openReorderDB(t, "file:"+filepath.Join(t.TempDir(), "t.db"))
	_, err := db.Exec(`CREATE TABLE t (ns TEXT NOT NULL, pkey TEXT NOT NULL, val BYTEA NOT NULL DEFAULT '')`)
	require.NoError(t, err)

	require.ErrorContains(t, ReorderPrimaryKey(db, db, "t", "ns, pkey, val", textStaging), "has no primary key")
}

// A rebuild that fails partway rolls back: the staging table is gone, and the
// table keeps its rows and its (pkey, ns) key.
func TestReorderFailureKeepsTable(t *testing.T) {
	t.Parallel()

	db := openReorderDB(t, "file:"+filepath.Join(t.TempDir(), "t.db"))
	// SQLite allows NULL in a non-integer primary key unless NOT NULL is
	// declared, so the copy into the staging table, which declares it, fails
	// after that table has been created.
	_, err := db.Exec(`CREATE TABLE t (
		ns TEXT, pkey TEXT, val BYTEA,
		PRIMARY KEY (pkey, ns)
	)`)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t (ns, pkey, val) VALUES ('ns', 'k1', 'v'), (NULL, 'k2', 'v')")
	require.NoError(t, err)

	require.ErrorContains(t, ReorderPrimaryKey(db, db, "t", "ns, pkey, val", textStaging), "failed reordering primary key")
	cols, err := PrimaryKeyColumns(db, "t")
	require.NoError(t, err)
	require.Equal(t, "pkey,ns", cols)
	var n int
	require.NoError(t, db.QueryRow("SELECT count(*) FROM t").Scan(&n))
	require.Equal(t, 2, n)
	require.NoError(t, db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 't_fsc_nskey'").Scan(&n))
	require.Zero(t, n)
}

// Two openers of one file both finish the rebuild.
func TestReorderPrimaryKeyConcurrently(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "t.db")
	first := openReorderDB(t, "file:"+path+"?_pragma=busy_timeout(5000)")
	createKeyedTable(t, first, "PRIMARY KEY (pkey, ns)")
	_, err := first.Exec("INSERT INTO t (ns, pkey, val) VALUES ('ns', 'k1', 'v')")
	require.NoError(t, err)

	second := openReorderDB(t, "file:"+path+"?_pragma=busy_timeout(5000)&mode=rw")
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Go(func() { errs[0] = ReorderPrimaryKey(first, first, "t", "ns, pkey, val", textStaging) })
	wg.Go(func() { errs[1] = ReorderPrimaryKey(second, second, "t", "ns, pkey, val", textStaging) })
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	cols, err := PrimaryKeyColumns(first, "t")
	require.NoError(t, err)
	require.Equal(t, "ns,pkey", cols)
	var val string
	require.NoError(t, first.QueryRow("SELECT val FROM t WHERE ns = 'ns' AND pkey = 'k1'").Scan(&val))
	require.Equal(t, "v", val)
}

// A replica that finds the write lock held past its busy_timeout fails with
// SQLITE_BUSY and leaves the table unchanged, rather than waiting indefinitely.
func TestReorderFailsWhenBusy(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "t.db")
	holder := openReorderDB(t, "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	createKeyedTable(t, holder, "PRIMARY KEY (pkey, ns)")
	tx, err := holder.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.Exec("UPDATE t SET ns = ns WHERE 0")
	require.NoError(t, err)

	db := openReorderDB(t, "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(50)")
	done := make(chan error, 1)
	go func() { done <- ReorderPrimaryKey(db, db, "t", "ns, pkey, val", textStaging) }()
	select {
	case err := <-done:
		require.ErrorContains(t, err, "SQLITE_BUSY")
	case <-time.After(10 * time.Second):
		t.Fatal("ReorderPrimaryKey did not return while the write lock was held")
	}
	require.NoError(t, tx.Rollback())
	cols, err := PrimaryKeyColumns(db, "t")
	require.NoError(t, err)
	require.Equal(t, "pkey,ns", cols)
	require.NoError(t, ReorderPrimaryKey(db, db, "t", "ns, pkey, val", textStaging))
	cols, err = PrimaryKeyColumns(db, "t")
	require.NoError(t, err)
	require.Equal(t, "ns,pkey", cols)
}
