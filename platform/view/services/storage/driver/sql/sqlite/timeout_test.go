/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
)

// TestKVSQueryTimeoutIsPerPersistence checks that queryTimeout reaches a store built
// through NewPersistenceWithOpts, and only the store of its own persistence: two
// persistences on one data source share the pools, not the timeout.
func TestKVSQueryTimeoutIsPerPersistence(t *testing.T) {
	t.Parallel()

	ds := tempDataSource(t, "kvs")
	provider := NewDbProvider()
	open := func(timeout time.Duration) driver.KeyValueStore {
		cp := NewConfigProvider(drivertesting.MockConfig(Config{DataSource: ds, QueryTimeout: timeout}))
		store, err := NewPersistenceWithOpts(cp, provider, "", NewKeyValueStore)
		require.NoError(t, err)
		return store
	}
	bounded := open(200 * time.Millisecond)
	unbounded := open(0)
	defer func() { _ = unbounded.Close() }()
	require.NoError(t, unbounded.SetState(t.Context(), "ns", "k", []byte("v")))

	// Another process holds the write lock longer than busy_timeout(1000) lets the
	// unbounded store wait.
	holder, err := sql.Open("sqlite", strings.Replace(ds, "busy_timeout(1000)", "journal_mode(WAL)", 1))
	require.NoError(t, err)
	defer func() { _ = holder.Close() }()
	tx, err := holder.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec("DELETE FROM fsc_kvs WHERE 0")
	require.NoError(t, err)

	start := time.Now()
	require.ErrorIs(t, bounded.SetState(context.Background(), "ns", "k", []byte("w")), context.DeadlineExceeded)
	require.Less(t, time.Since(start), 900*time.Millisecond)
	require.ErrorIs(t, unbounded.SetState(context.Background(), "ns", "k", []byte("w")), driver.SqlBusy)
}
