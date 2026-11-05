/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// TestPostgresVaultStoreQueryTimeout checks that queryTimeout ends a batch commit
// waiting for a lock another session holds, with an error wrapping
// context.DeadlineExceeded, and that the pool serves the next commit. The name
// carries "Postgres" so only the container suite runs it.
func TestPostgresVaultStoreQueryTimeout(t *testing.T) {
	t.Parallel()

	terminate, dsn, err := postgres.StartPostgres(t.Context(), postgres.ConfigFromEnv(), nil)
	require.NoError(t, err)
	t.Cleanup(terminate)
	cp := postgres.NewConfigProvider(drivertesting.MockConfig(postgres.Config{DataSource: dsn, QueryTimeout: 200 * time.Millisecond}))
	store, err := NewPersistenceWithOpts(cp, postgres.NewDbProvider(), "", NewVaultStore)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	writes := driver.Writes{"ns": {"k": {Raw: []byte("v"), Version: []byte("1")}}}

	lock, err := store.writeDB.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	_, err = lock.ExecContext(t.Context(), "LOCK TABLE "+store.tables.StateTable+" IN ACCESS EXCLUSIVE MODE")
	require.NoError(t, err)

	start := time.Now()
	err = store.Store(context.Background(), []driver.TxID{"tx1"}, writes, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 5*time.Second)

	// pgx ends the statement by closing the connection; the server keeps running it
	// until it notices, so the next commit writes another key.
	require.NoError(t, lock.Rollback())
	require.NoError(t, store.Store(context.Background(), []driver.TxID{"tx2"}, driver.Writes{"ns": {"k2": {Raw: []byte("v"), Version: []byte("1")}}}, nil))
}

// TestPostgresVaultStoreLockTimeoutIsBusy checks that a server-side lock_timeout
// is reported as dbdriver.SqlBusy, which the committer retries.
func TestPostgresVaultStoreLockTimeoutIsBusy(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStoreWithParams(t, "lock_timeout=200")
	lock, err := store.writeDB.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lock.Rollback() })
	_, err = lock.ExecContext(t.Context(), "LOCK TABLE "+store.tables.StateTable+" IN ACCESS EXCLUSIVE MODE")
	require.NoError(t, err)

	err = store.Store(t.Context(), []driver.TxID{"tx"}, driver.Writes{"ns": {"k": {Raw: []byte("v"), Version: []byte("1")}}}, nil)
	require.ErrorIs(t, err, dbdriver.SqlBusy)
}
