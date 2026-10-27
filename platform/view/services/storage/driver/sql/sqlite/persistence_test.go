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
)

// TestOpenSkipPragmas checks a database opens when the default pragmas are skipped, both
// with and without WAL mode set in the data source.
func TestOpenSkipPragmas(t *testing.T) {
	t.Parallel()

	t.Run("without WAL", func(t *testing.T) {
		t.Parallel()

		dbs, err := open(Opts{DataSource: tempDataSource(t, "nowal"), SkipPragmas: true})
		require.NoError(t, err)
		assert.NotNil(t, dbs)
	})

	t.Run("with WAL", func(t *testing.T) {
		t.Parallel()

		dbs, err := open(Opts{DataSource: tempDataSource(t, "wal") + "&_pragma=journal_mode(WAL)", SkipPragmas: true})
		require.NoError(t, err)
		assert.NotNil(t, dbs)
	})
}

// TestGetDirMalformedDataSource checks a data source that is not a parseable URL yields no
// directory rather than an error.
func TestGetDirMalformedDataSource(t *testing.T) {
	t.Parallel()

	assert.Empty(t, getDir("file:///tmp/%zz.sqlite"))
}

// pragmaInt returns the integer value of PRAGMA name on conn.
func pragmaInt(t *testing.T, conn *sql.Conn, name string) int {
	t.Helper()

	var v int
	require.NoError(t, conn.QueryRowContext(t.Context(), "PRAGMA "+name).Scan(&v))
	return v
}

// TestOpenAppliesPragmasToEveryConnection checks the default pragmas hold on every
// connection of the pool, and a pragma set in the data source takes precedence.
func TestOpenAppliesPragmasToEveryConnection(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		dataSource  string
		busyTimeout int
	}{
		"plain path":         {dataSource: filepath.Join(t.TempDir(), "plain.sqlite"), busyTimeout: 5000},
		"file uri":           {dataSource: "file:" + filepath.Join(t.TempDir(), "uri.sqlite"), busyTimeout: 5000},
		"pragma in dsn wins": {dataSource: tempDataSource(t, "dsn"), busyTimeout: 1000},
		"shorthand in dsn":   {dataSource: "file:" + filepath.Join(t.TempDir(), "short.sqlite") + "?_busy_timeout=2000", busyTimeout: 2000},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			const conns = 4
			dbs, err := open(Opts{DataSource: tc.dataSource, MaxOpenConns: conns})
			require.NoError(t, err)
			t.Cleanup(func() { _ = dbs.ReadDB.Close(); _ = dbs.WriteDB.Close() })

			for i := range conns {
				conn, err := dbs.ReadDB.Conn(t.Context())
				require.NoError(t, err)
				t.Cleanup(func() { _ = conn.Close() })

				assert.Equal(t, tc.busyTimeout, pragmaInt(t, conn, "busy_timeout"), "busy_timeout on connection %d", i)
				assert.Equal(t, 1, pragmaInt(t, conn, "foreign_keys"), "foreign_keys on connection %d", i)
				assert.Equal(t, 1, pragmaInt(t, conn, "synchronous"), "synchronous on connection %d", i)
				assert.Equal(t, 2, pragmaInt(t, conn, "temp_store"), "temp_store on connection %d", i)
			}
		})
	}
}

// TestWithDefaultPragmas checks the defaults are appended with the right separator
// and a default the data source already sets is not added again.
func TestWithDefaultPragmas(t *testing.T) {
	t.Parallel()

	const all = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=temp_store(memory)&_pragma=foreign_keys(ON)"
	for in, want := range map[string]string{
		"/db.sqlite":      "/db.sqlite?" + all,
		"file:/db.sqlite": "file:/db.sqlite?" + all,
		"file:/db.sqlite?_txlock=immediate&_pragma=Busy_Timeout(1)&_journal=DELETE&_pragma=foreign_keys=off": "file:/db.sqlite?_txlock=immediate&_pragma=Busy_Timeout(1)&_journal=DELETE&_pragma=foreign_keys=off" +
			"&_pragma=synchronous(NORMAL)&_pragma=temp_store(memory)",
		"file:/db.sqlite?_pragma=main.synchronous(OFF)": "file:/db.sqlite?_pragma=main.synchronous(OFF)" +
			"&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=temp_store(memory)&_pragma=foreign_keys(ON)",
		"file:/db.sqlite?%zz": "file:/db.sqlite?%zz",
	} {
		assert.Equal(t, want, withDefaultPragmas(in), in)
	}
}
