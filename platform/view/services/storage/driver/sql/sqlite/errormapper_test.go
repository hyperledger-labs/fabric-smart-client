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
