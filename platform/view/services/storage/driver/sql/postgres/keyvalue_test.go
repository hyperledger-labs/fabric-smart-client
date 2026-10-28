/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

func openPostgresKVS(t *testing.T, pgConnStr string, skipCreate bool) *KeyValueStore {
	t.Helper()
	cp := NewConfigProvider(drivertesting.MockConfig(Config{
		DataSource:      pgConnStr,
		SkipCreateTable: skipCreate,
	}))
	store, err := NewPersistenceWithOpts(cp, NewDbProvider(), "", NewKeyValueStore)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// CreateSchema reorders a key-value table whose primary key is (pkey, ns).
// Rows in two namespaces that share keys stay in the namespace they were
// written to, and a second call does nothing.
func TestPostgresKVSReordersPrimaryKey(t *testing.T) {
	t.Parallel()

	store := openPostgresKVS(t, setupDB(t), true)
	ctx := t.Context()
	table := store.table
	_, err := store.writeDB.Exec(`CREATE TABLE ` + table + ` (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		CONSTRAINT kvs_old_pk PRIMARY KEY (pkey, ns)
	)`)
	require.NoError(t, err)
	_, err = store.writeDB.Exec("INSERT INTO "+table+" (ns, pkey, val) VALUES ($1, $2, $3), ($4, $5, $6), ($7, $8, $9)",
		"ns-a", []byte("k2"), []byte("a2"),
		"ns-a", []byte("k1"), []byte("a1"),
		"ns-b", []byte("k1"), []byte("b1"),
	)
	require.NoError(t, err)

	require.NoError(t, store.CreateSchema())
	_, cols, err := PrimaryKey(store.writeDB, table)
	require.NoError(t, err)
	require.Equal(t, "ns,pkey", cols)
	var left int
	require.NoError(t, store.writeDB.QueryRow("SELECT count(*) FROM pg_constraint WHERE conname = 'kvs_old_pk'").Scan(&left))
	require.Zero(t, left)
	require.NoError(t, store.CreateSchema())

	got, err := store.GetState(ctx, "ns-b", "k1")
	require.NoError(t, err)
	require.Equal(t, []byte("b1"), got)
	it, err := store.GetStateRangeScanIterator(ctx, "ns-a", "k1", "k3")
	require.NoError(t, err)
	reads, err := collections.ReadAll(it)
	require.NoError(t, err)
	require.Equal(t, []string{"k1", "k2"}, []string{reads[0].Key, reads[1].Key})
}

// GetStateRangeScanIterator scans the (ns, pkey) index from the namespace and
// does not sort.
func TestPostgresKVSRangePlanUsesNamespaceKey(t *testing.T) {
	t.Parallel()

	store := openPostgresKVS(t, setupDB(t), false)
	ctx := t.Context()
	for _, ns := range []string{"ns-a", "ns-b"} {
		require.NoError(t, store.BeginUpdate())
		for _, k := range []string{"k1", "k2", "k3"} {
			require.NoError(t, store.SetState(ctx, ns, k, []byte(k)))
		}
		require.NoError(t, store.Commit())
	}

	tx, err := store.writeDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.Exec("SET LOCAL enable_seqscan = off")
	require.NoError(t, err)
	_, err = tx.Exec("SET LOCAL enable_bitmapscan = off")
	require.NoError(t, err)
	rows, err := tx.Query("EXPLAIN SELECT pkey, val FROM "+store.table+
		" WHERE (ns = $1 AND (pkey >= $2 AND pkey < $3)) ORDER BY pkey ASC", "ns-a", []byte("k1"), []byte("k3"))
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	require.NoError(t, rows.Err())
	require.Contains(t, plan.String(), "Index Cond: ((ns = ")
	require.NotContains(t, plan.String(), "Sort")
}

// Dropping and re-adding the primary key leaves the notify trigger in place,
// because the trigger reads ns and pkey from the row.
func TestPostgresKVSNotifierAfterReorder(t *testing.T) {
	t.Parallel()

	pgConnStr := setupDB(t)
	cp := NewConfigProvider(drivertesting.MockConfig(Config{
		DataSource:      pgConnStr,
		SkipCreateTable: true,
	}))
	store, err := NewPersistenceWithOpts(cp, NewDbProvider(), "", func(dbs *common.RWDB, tables sqlcommon.TableNames) (*KeyValueStoreNotifier, error) {
		return &KeyValueStoreNotifier{
			KeyValueStore: buildKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS),
			Notifier:      NewNotifier(dbs.WriteDB, tables.KVS, pgConnStr, AllOperations, *NewSimplePrimaryKey("ns"), *NewBytePrimaryKey("pkey")),
		}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	table := store.KeyValueStore.table
	_, err = store.KeyValueStore.writeDB.Exec(`CREATE TABLE ` + table + ` (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		PRIMARY KEY (pkey, ns)
	)`)
	require.NoError(t, err)
	_, err = store.KeyValueStore.writeDB.Exec("INSERT INTO "+table+" (ns, pkey, val) VALUES ($1, $2, $3)", "ns", []byte("k1"), []byte("v1"))
	require.NoError(t, err)
	require.NoError(t, store.CreateSchema())

	ch := make(chan driver.Operation, 1)
	require.NoError(t, store.Subscribe(func(op driver.Operation, m map[driver.ColumnKey]string) {
		if m["ns"] == "ns" {
			select {
			case ch <- op:
			default:
			}
		}
	}))

	// Subscribe starts listening in the background, so a write can land before
	// LISTEN runs. Retry the write until a notification arrives.
	ctx := t.Context()
	require.Eventually(t, func() bool {
		if store.BeginUpdate() != nil || store.SetState(ctx, "ns", "k2", []byte("v2")) != nil || store.Commit() != nil {
			return false
		}
		select {
		case op := <-ch:
			return op == driver.Insert || op == driver.Update
		case <-time.After(100 * time.Millisecond):
			return false
		}
	}, 10*time.Second, 10*time.Millisecond, "no notification after reorder")
	got, err := store.GetState(ctx, "ns", "k1")
	require.NoError(t, err)
	require.Equal(t, []byte("v1"), got)
}
