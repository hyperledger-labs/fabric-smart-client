/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

// GetStateRange returns keys sorted in byte order even when SQLite returns
// unordered rows in reverse, which reverse_unordered_selects forces on every
// pooled connection.
func TestVaultStoreRangeIsSorted(t *testing.T) {
	t.Parallel()

	store := openSQLiteVault(t, "file:"+filepath.Join(t.TempDir(), "vault.db")+"?_pragma=reverse_unordered_selects(1)", false)
	ctx := t.Context()
	// One Store per key, in key order, so rowid and index order agree and the
	// reversed scan is unsorted whichever the planner picks.
	for _, k := range []driver.PKey{"k1", "k111", "k2", "k3"} {
		require.NoError(t, store.Store(ctx, nil, driver.Writes{"ns": {k: {Raw: []byte(k), Version: []byte("v")}}}, nil))
	}

	it, err := store.GetStateRange(ctx, "ns", "k1", "k3")
	require.NoError(t, err)
	require.Equal(t, []driver.PKey{"k1", "k111", "k2"}, rangeKeys(t, it))
}

func openSQLiteVault(t *testing.T, dataSource string, skipCreate bool) *VaultStore {
	t.Helper()
	cp := sqlite2.NewConfigProvider(testing2.MockConfig(sqlite2.Config{
		DataSource:      dataSource,
		SkipCreateTable: skipCreate,
	}))
	store, err := NewPersistenceWithOpts(cp, sqlite2.NewDbProvider(), "", NewVaultStore)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// recreateStateTable replaces the store's state table with one declaring
// primaryKey, so tests can start from a layout CreateSchema no longer creates.
func recreateStateTable(t *testing.T, store *VaultStore, primaryKey string) {
	t.Helper()
	table := store.tables.StateTable
	_, err := store.writeDB.ExecContext(t.Context(), "DROP TABLE "+table)
	require.NoError(t, err)
	_, err = store.writeDB.ExecContext(t.Context(), `CREATE TABLE `+table+` (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA DEFAULT '',
		kversion BYTEA DEFAULT '',
		metadata BYTEA DEFAULT '',
		`+primaryKey+`
	)`)
	require.NoError(t, err)
}

func rangeKeys(t *testing.T, it iterators.Iterator[*driver.VaultRead]) []driver.PKey {
	t.Helper()
	reads, err := collections.ReadAll(it)
	require.NoError(t, err)
	keys := make([]driver.PKey, len(reads))
	for i, r := range reads {
		keys[i] = r.Key
	}
	return keys
}

func requireSQLitePrimaryKey(t *testing.T, store *VaultStore, want string) {
	t.Helper()
	got, err := sqlite2.PrimaryKeyColumns(store.readDB, store.tables.StateTable)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func requireState(t *testing.T, store *VaultStore, ns driver.Namespace, key driver.PKey, want string) {
	t.Helper()
	read, err := store.GetState(t.Context(), ns, key)
	require.NoError(t, err)
	require.NotNil(t, read)
	require.Equal(t, driver.RawValue(want), read.Raw)
}

// CreateSchema rebuilds a table whose primary key is (pkey, ns). Rows in two
// namespaces that share keys stay in the namespace they were written to, and a
// second call does nothing.
func TestVaultStoreReordersPrimaryKey(t *testing.T) {
	t.Parallel()

	store := openSQLiteVault(t, "file:"+filepath.Join(t.TempDir(), "vault.db"), false)
	ctx := t.Context()
	recreateStateTable(t, store, "PRIMARY KEY (pkey, ns)")
	require.NoError(t, store.Store(ctx, nil, driver.Writes{
		"ns-a": {
			"k2":                 {Raw: []byte("a2"), Version: []byte("v")},
			"k1":                 {Raw: []byte("a1"), Version: []byte("v")},
			"\x00Token\x00alice": {Raw: []byte("tok"), Version: []byte("v")},
		},
		"ns-b": {
			"k1": {Raw: []byte("b1"), Version: []byte("v")},
			"k2": {Raw: []byte("b2"), Version: []byte("v")},
		},
	}, nil))

	require.NoError(t, store.CreateSchema())
	requireSQLitePrimaryKey(t, store, "ns,pkey")
	require.NoError(t, store.CreateSchema())

	requireState(t, store, "ns-a", "\x00Token\x00alice", "tok")
	requireState(t, store, "ns-b", "k1", "b1")
	it, err := store.GetStateRange(ctx, "ns-a", "k1", "k3")
	require.NoError(t, err)
	require.Equal(t, []driver.PKey{"k1", "k2"}, rangeKeys(t, it))
}

// GetStateRange uses the (ns, pkey) index and does not sort.
func TestVaultStoreRangePlanUsesNamespaceKey(t *testing.T) {
	t.Parallel()

	store := openSQLiteVault(t, "file:"+filepath.Join(t.TempDir(), "vault.db"), false)
	ctx := t.Context()
	for _, ns := range []string{"ns-a", "ns-b"} {
		for _, k := range []string{"k1", "k2", "k3"} {
			require.NoError(t, store.Store(ctx, nil, driver.Writes{
				ns: {k: {Raw: []byte(k), Version: []byte("v")}},
			}, nil))
		}
	}

	rows, err := store.readDB.QueryContext(ctx, "EXPLAIN QUERY PLAN SELECT pkey, kversion, val FROM "+store.tables.StateTable+
		" WHERE (ns = $1 AND (pkey >= $2 AND pkey < $3)) ORDER BY pkey ASC", "ns-a", "k1", "k3")
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
