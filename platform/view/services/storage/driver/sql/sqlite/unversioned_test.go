/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
)

func openSQLiteKVS(t *testing.T, dataSource string, skipCreate bool) *KeyValueStore {
	t.Helper()
	store, err := NewPersistenceWithOpts(NewConfigProvider(testing2.MockConfig(Config{
		DataSource:      dataSource,
		SkipCreateTable: skipCreate,
	})), NewDbProvider(), "", NewKeyValueStore)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// CreateSchema rebuilds a key-value table whose primary key is (pkey, ns).
// Rows in two namespaces that share keys stay in the namespace they were
// written to, and a second call does nothing.
func TestKVSReordersPrimaryKey(t *testing.T) {
	t.Parallel()

	store := openSQLiteKVS(t, "file:"+filepath.Join(t.TempDir(), "kvs.db"), false)
	ctx := t.Context()
	table := store.table
	_, err := store.writeDB.Exec("DROP TABLE " + table)
	require.NoError(t, err)
	_, err = store.writeDB.Exec(`CREATE TABLE ` + table + ` (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		PRIMARY KEY (pkey, ns)
	)`)
	require.NoError(t, err)
	_, err = store.writeDB.Exec("INSERT INTO "+table+" (ns, pkey, val) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)",
		"ns-a", []byte("k2"), []byte("a2"),
		"ns-a", []byte("k1"), []byte("a1"),
		"ns-b", []byte("k1"), []byte("b1"),
	)
	require.NoError(t, err)

	require.NoError(t, store.CreateSchema())
	cols, err := PrimaryKeyColumns(store.readDB, table)
	require.NoError(t, err)
	require.Equal(t, "ns,pkey", cols)
	require.NoError(t, store.CreateSchema())

	got, err := store.GetState(ctx, "ns-b", "k1")
	require.NoError(t, err)
	require.Equal(t, []byte("b1"), []byte(got))
	it, err := store.GetStateRangeScanIterator(ctx, "ns-a", "k1", "k3")
	require.NoError(t, err)
	reads, err := collections.ReadAll(it)
	require.NoError(t, err)
	require.Equal(t, []string{"k1", "k2"}, []string{reads[0].Key, reads[1].Key})
}

// GetStateRangeScanIterator uses the (ns, pkey) index and does not sort.
func TestKVSRangePlanUsesNamespaceKey(t *testing.T) {
	t.Parallel()

	store := openSQLiteKVS(t, "file:"+filepath.Join(t.TempDir(), "kvs.db"), false)
	ctx := context.Background()
	for _, ns := range []string{"ns-a", "ns-b"} {
		require.NoError(t, store.BeginUpdate())
		for _, k := range []string{"k1", "k2", "k3"} {
			require.NoError(t, store.SetState(ctx, ns, k, []byte(k)))
		}
		require.NoError(t, store.Commit())
	}

	rows, err := store.readDB.QueryContext(ctx, "EXPLAIN QUERY PLAN SELECT pkey, val FROM "+store.table+
		" WHERE (ns = $1 AND (pkey >= $2 AND pkey < $3)) ORDER BY pkey ASC", "ns-a", []byte("k1"), []byte("k3"))
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var plan string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notused, &detail))
		plan += detail + "\n"
	}
	require.NoError(t, rows.Err())
	require.Contains(t, plan, "ns=")
	require.NotContains(t, plan, "USE TEMP B-TREE FOR ORDER BY")
	require.NotContains(t, plan, "SCAN")
}
