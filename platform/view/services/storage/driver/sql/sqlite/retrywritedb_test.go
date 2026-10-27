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

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// TestRetryWriteDBExec checks Exec runs a statement, and an error that is not SQLITE_BUSY
// is returned without a retry.
func TestRetryWriteDBExec(t *testing.T) {
	t.Parallel()

	db := NewRetryWriteDB(openTestDB(t))

	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)

	_, err = db.Exec("NOT VALID SQL")
	require.Error(t, err)
}

// busyHook is an SQLErrorWrapper that counts the SQLITE_BUSY errors it maps and
// runs onBusy on the first one, so a test can release a lock between two attempts.
type busyHook struct {
	busy   int
	onBusy func()
}

func (h *busyHook) WrapError(err error) error {
	err = (&ErrorMapper{}).WrapError(err)
	if errors.Is(err, driver.SqlBusy) {
		h.busy++
		if h.busy == 1 && h.onBusy != nil {
			h.onBusy()
		}
	}
	return err
}

// lockedDB opens a database whose connections do not wait for locks and holds the
// write lock on it from a read connection. It returns the write DB and the
// connection holding the lock.
func lockedDB(t *testing.T) (*sql.DB, *sql.Conn) {
	t.Helper()

	dbs, err := open(Opts{DataSource: "file:" + filepath.Join(t.TempDir(), "busy.sqlite") + "?_pragma=busy_timeout(0)"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = dbs.ReadDB.Close(); _ = dbs.WriteDB.Close() })
	_, err = dbs.WriteDB.Exec("CREATE TABLE t (id INTEGER)")
	require.NoError(t, err)

	locker, err := dbs.ReadDB.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = locker.Close() })
	_, err = locker.ExecContext(t.Context(), "BEGIN EXCLUSIVE")
	require.NoError(t, err)

	return dbs.WriteDB, locker
}

// TestRetryWriteDBBusy checks a write rejected with SQLITE_BUSY is retried: it
// succeeds once the lock is released between attempts, and returns the busy error
// after maxBusyRetries retries when the lock is held throughout.
func TestRetryWriteDBBusy(t *testing.T) {
	t.Parallel()

	t.Run("lock released", func(t *testing.T) {
		t.Parallel()

		writeDB, locker := lockedDB(t)
		hook := &busyHook{onBusy: func() {
			_, err := locker.ExecContext(t.Context(), "ROLLBACK")
			require.NoError(t, err)
		}}
		db := &retryWriteDB{DB: writeDB, errorWrapper: hook}

		_, err := db.Exec("INSERT INTO t VALUES (1)")
		require.NoError(t, err)
		assert.Equal(t, 1, hook.busy)
	})

	t.Run("lock held", func(t *testing.T) {
		t.Parallel()

		writeDB, _ := lockedDB(t)
		hook := &busyHook{}
		db := &retryWriteDB{DB: writeDB, errorWrapper: hook}

		_, err := db.Exec("INSERT INTO t VALUES (1)")
		require.ErrorIs(t, (&ErrorMapper{}).WrapError(err), driver.SqlBusy)
		assert.Equal(t, maxBusyRetries, hook.busy)
	})
}
