/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

const testQueryTimeout = 500 * time.Millisecond

func openTimeoutVault(t *testing.T, dataSource string) *VaultStore {
	t.Helper()
	cp := sqlite.NewConfigProvider(drivertesting.MockConfig(sqlite.Config{DataSource: dataSource, QueryTimeout: testQueryTimeout}))
	store, err := NewPersistenceWithOpts(cp, sqlite.NewDbProvider(), "", NewVaultStore)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// holdWriteLock takes the write lock of the database at path from a second pool,
// as another process would, until the test ends.
func holdWriteLock(t *testing.T, path, table string) {
	t.Helper()
	holder, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Close() })
	tx, err := holder.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.Exec("UPDATE " + table + " SET ns = ns WHERE 0")
	require.NoError(t, err)
}

var timeoutWrites = driver.Writes{"ns": {"k": {Raw: []byte("v"), Version: []byte("1")}}}

// A batch commit that waits for a lock held elsewhere ends at the query timeout,
// even when the caller's context has no deadline and busy_timeout is longer.
func TestVaultStoreQueryTimeoutBoundsStore(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "v.db")
	store := openTimeoutVault(t, "file:"+path+"?_pragma=busy_timeout(10000)")
	holdWriteLock(t, path, store.tables.StateTable)

	start := time.Now()
	err := store.Store(context.Background(), []driver.TxID{"tx"}, timeoutWrites, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 5*time.Second)
}

// SQLITE_BUSY from a batch commit is mapped, so the committer retries it.
func TestVaultStoreStoreMapsBusy(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "v.db")
	store := openSQLiteVault(t, "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10)", false)
	holdWriteLock(t, path, store.tables.StateTable)

	err := store.Store(t.Context(), []driver.TxID{"tx"}, timeoutWrites, nil)
	require.ErrorIs(t, err, dbdriver.SqlBusy)
}

// An iterator keeps its statement's deadline after the query function returned:
// it reads to the end within the timeout, and fails once the timeout expires.
func TestVaultStoreQueryTimeoutIterator(t *testing.T) {
	t.Parallel()

	store := openTimeoutVault(t, "file:"+filepath.Join(t.TempDir(), "v.db"))
	ctx := t.Context()
	for _, k := range []driver.PKey{"k1", "k2", "k3"} {
		require.NoError(t, store.Store(ctx, nil, driver.Writes{"ns": {k: {Raw: []byte(k), Version: []byte("v")}}}, nil))
	}

	it, err := store.GetStateRange(ctx, "ns", "", "")
	require.NoError(t, err)
	require.Equal(t, []driver.PKey{"k1", "k2", "k3"}, rangeKeys(t, it))
	it.Close()

	it, err = store.GetStateRange(ctx, "ns", "", "")
	require.NoError(t, err)
	defer it.Close()
	_, err = it.Next()
	require.NoError(t, err)
	time.Sleep(2 * testQueryTimeout)
	var next *driver.VaultRead
	for err == nil {
		next, err = it.Next()
		if next == nil && err == nil {
			t.Fatal("iterator ended without an error after its deadline")
		}
	}
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// A TxLock reader held longer than the query timeout still reads: its transaction
// has no deadline, only each statement has.
func TestVaultStoreQueryTimeoutTxLockReader(t *testing.T) {
	t.Parallel()

	store := openTimeoutVault(t, "file:"+filepath.Join(t.TempDir(), "v.db"))
	ctx := t.Context()
	require.NoError(t, store.Store(ctx, nil, timeoutWrites, nil))

	r, err := store.NewTxLockVaultReader(ctx, "tx", driver.LevelDefault)
	require.NoError(t, err)
	_, err = r.GetState(ctx, "ns", "k")
	require.NoError(t, err)
	time.Sleep(2 * testQueryTimeout)
	v, err := r.GetState(ctx, "ns", "k")
	require.NoError(t, err)
	require.Equal(t, []byte("v"), v.Raw)
	require.NoError(t, r.Done())
}
