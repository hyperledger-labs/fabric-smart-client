/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"modernc.org/sqlite"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// openTestDB opens a fresh sqlite database under t's temp dir.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return db
}

// TestWrapErrorPassesThroughNonSqliteErrors checks an error that is not from sqlite is
// returned unchanged.
func TestWrapErrorPassesThroughNonSqliteErrors(t *testing.T) {
	t.Parallel()

	err := errors.New("not sqlite")

	assert.Equal(t, err, (&ErrorMapper{}).WrapError(err))
}

// TestWrapErrorMapsUniqueViolation checks a duplicate primary key maps to
// driver.UniqueKeyViolation.
func TestWrapErrorMapsUniqueViolation(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t (id) VALUES (1)")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO t (id) VALUES (1)")
	require.Error(t, err)

	require.ErrorIs(t, (&ErrorMapper{}).WrapError(err), driver.UniqueKeyViolation)
}

// TestWrapErrorLeavesUnmappedCodes checks a sqlite error with no mapping is returned as
// the sqlite error rather than a driver error.
func TestWrapErrorLeavesUnmappedCodes(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	_, err := db.Exec("NOT VALID SQL")
	require.Error(t, err)

	wrapped := (&ErrorMapper{}).WrapError(err)

	var sqliteErr *sqlite.Error
	require.ErrorAs(t, wrapped, &sqliteErr, "still a sqlite error")
	require.NotErrorIs(t, wrapped, driver.UniqueKeyViolation)
	assert.NotErrorIs(t, wrapped, driver.SqlBusy)
}

// TestWrapErrorMapsBusySnapshot checks SQLITE_BUSY_SNAPSHOT (517) maps to
// driver.SqlBusy. A read transaction that tries to write after another connection
// committed gets 517 at once: SQLite does not call the busy handler for it.
func TestWrapErrorMapsBusySnapshot(t *testing.T) {
	t.Parallel()

	dbs, err := open(Opts{DataSource: tempDataSource(t, "snapshot"), MaxOpenConns: 1})
	require.NoError(t, err)
	t.Cleanup(func() { _ = dbs.ReadDB.Close(); _ = dbs.WriteDB.Close() })
	_, err = dbs.WriteDB.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)

	tx, err := dbs.ReadDB.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	var n int
	require.NoError(t, tx.QueryRow("SELECT COUNT(*) FROM t").Scan(&n))

	_, err = dbs.WriteDB.Exec("INSERT INTO t (id) VALUES (1)")
	require.NoError(t, err)

	_, err = tx.Exec("INSERT INTO t (id) VALUES (2)")
	var sqliteErr *sqlite.Error
	require.ErrorAs(t, err, &sqliteErr)
	require.Equal(t, 517, sqliteErr.Code())

	assert.ErrorIs(t, (&ErrorMapper{}).WrapError(err), driver.SqlBusy)
}

// TestSentinelForFallsBackToPrimaryCode checks an extended code without its own entry
// maps by its primary code, and a constraint failure other than a unique or primary
// key violation does not map to driver.UniqueKeyViolation.
func TestSentinelForFallsBackToPrimaryCode(t *testing.T) {
	t.Parallel()

	for code, want := range map[int]error{
		5:    driver.SqlBusy,            // SQLITE_BUSY
		261:  driver.SqlBusy,            // SQLITE_BUSY_RECOVERY
		517:  driver.SqlBusy,            // SQLITE_BUSY_SNAPSHOT
		773:  driver.SqlBusy,            // SQLITE_BUSY_TIMEOUT
		1555: driver.UniqueKeyViolation, // SQLITE_CONSTRAINT_PRIMARYKEY
		2067: driver.UniqueKeyViolation, // SQLITE_CONSTRAINT_UNIQUE
		19:   nil,                       // SQLITE_CONSTRAINT
		787:  nil,                       // SQLITE_CONSTRAINT_FOREIGNKEY
		1:    nil,                       // SQLITE_ERROR
	} {
		assert.Equal(t, want, sentinelFor(code), "code %d", code)
	}
}
