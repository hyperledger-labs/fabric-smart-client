/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

// GetStateRange returns keys sorted in byte order even when SQLite returns
// unordered rows in reverse, which reverse_unordered_selects forces on every
// pooled connection.
func TestVaultStoreRangeIsSorted(t *testing.T) {
	t.Parallel()

	cp := sqlite2.NewConfigProvider(testing2.MockConfig(sqlite2.Config{
		DataSource: "file:" + filepath.Join(t.TempDir(), "vault.db") + "?_pragma=reverse_unordered_selects(1)",
	}))
	store, err := NewPersistenceWithOpts(cp, sqlite2.NewDbProvider(), "", NewVaultStore)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	// One Store per key, in key order, so rowid and index order agree and the
	// reversed scan is unsorted whichever the planner picks.
	for _, k := range []driver.PKey{"k1", "k111", "k2", "k3"} {
		require.NoError(t, store.Store(ctx, nil, driver.Writes{"ns": {k: {Raw: []byte(k), Version: []byte("v")}}}, nil))
	}

	it, err := store.GetStateRange(ctx, "ns", "k1", "k3")
	require.NoError(t, err)
	reads, err := collections.ReadAll(it)
	require.NoError(t, err)
	got := make([]driver.PKey, len(reads))
	for i, r := range reads {
		got[i] = r.Key
	}
	require.Equal(t, []driver.PKey{"k1", "k111", "k2"}, got)
}

func newSQLiteVault(t *testing.T, dataSource string) *VaultStore {
	t.Helper()
	return openSQLiteVault(t, dataSource, false)
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

func requireSQLitePrimaryKey(t *testing.T, store *VaultStore, want string) {
	t.Helper()
	got, err := store.primaryKeyColumns(store.readDB)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// CreateSchema rebuilds a table whose primary key is (pkey, ns). Rows in two
// namespaces that share keys stay in the namespace they were written to, and a
// second call does nothing.
func TestVaultStoreReordersPrimaryKey(t *testing.T) {
	t.Parallel()

	store := newSQLiteVault(t, "file:"+filepath.Join(t.TempDir(), "vault.db"))
	ctx := t.Context()
	table := store.tables.StateTable
	_, err := store.writeDB.ExecContext(ctx, "DROP TABLE "+table)
	require.NoError(t, err)
	_, err = store.writeDB.ExecContext(ctx, `CREATE TABLE `+table+` (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA DEFAULT '',
		kversion BYTEA DEFAULT '',
		metadata BYTEA DEFAULT '',
		PRIMARY KEY (pkey, ns)
	)`)
	require.NoError(t, err)
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

	read, err := store.GetState(ctx, "ns-a", "\x00Token\x00alice")
	require.NoError(t, err)
	require.NotNil(t, read)
	require.Equal(t, driver.RawValue("tok"), read.Raw)
	read, err = store.GetState(ctx, "ns-b", "k1")
	require.NoError(t, err)
	require.Equal(t, driver.RawValue("b1"), read.Raw)

	it, err := store.GetStateRange(ctx, "ns-a", "k1", "k3")
	require.NoError(t, err)
	reads, err := collections.ReadAll(it)
	require.NoError(t, err)
	got := make([]driver.PKey, len(reads))
	for i, r := range reads {
		got[i] = r.Key
	}
	require.Equal(t, []driver.PKey{"k1", "k2"}, got)
}

// A primary key other than (pkey, ns) or (ns, pkey) is not rewritten.
func TestVaultStoreRejectsOtherPrimaryKey(t *testing.T) {
	t.Parallel()

	store := newSQLiteVault(t, "file:"+filepath.Join(t.TempDir(), "vault.db"))
	ctx := t.Context()
	table := store.tables.StateTable
	_, err := store.writeDB.ExecContext(ctx, "DROP TABLE "+table)
	require.NoError(t, err)
	_, err = store.writeDB.ExecContext(ctx, `CREATE TABLE `+table+` (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA DEFAULT '',
		kversion BYTEA DEFAULT '',
		metadata BYTEA DEFAULT '',
		PRIMARY KEY (pkey)
	)`)
	require.NoError(t, err)
	_, err = store.writeDB.ExecContext(ctx, "INSERT INTO "+table+" (ns, pkey, val, kversion) VALUES ('ns', 'k1', 'v', 'v')")
	require.NoError(t, err)

	err = store.CreateSchema()
	require.ErrorContains(t, err, "primary key is (pkey), expected (ns, pkey)")
	requireSQLitePrimaryKey(t, store, "pkey")
	read, err := store.GetState(ctx, "ns", "k1")
	require.NoError(t, err)
	require.NotNil(t, read)
	require.Equal(t, driver.RawValue("v"), read.Raw)
}

// GetStateRange uses the (ns, pkey) index and does not sort.
func TestVaultStoreRangePlanUsesNamespaceKey(t *testing.T) {
	t.Parallel()

	store := newSQLiteVault(t, "file:"+filepath.Join(t.TempDir(), "vault.db"))
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
	defer rows.Close()
	cols, err := rows.Columns()
	require.NoError(t, err)
	var plan string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		require.NoError(t, rows.Scan(ptrs...))
		for _, v := range vals {
			plan += fmt.Sprint(v) + " "
		}
		plan += "\n"
	}
	require.NoError(t, rows.Err())
	require.Contains(t, plan, "ns=")
	require.NotContains(t, plan, "USE TEMP B-TREE FOR ORDER BY")
	require.NotContains(t, plan, "SCAN")
}

// Two openers of one file both finish the rebuild.
func TestVaultStoreReordersPrimaryKeyConcurrently(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "vault.db")
	first := newSQLiteVault(t, "file:"+path+"?_pragma=busy_timeout(5000)")
	ctx := t.Context()
	table := first.tables.StateTable
	_, err := first.writeDB.ExecContext(ctx, "DROP TABLE "+table)
	require.NoError(t, err)
	_, err = first.writeDB.ExecContext(ctx, `CREATE TABLE `+table+` (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA DEFAULT '',
		kversion BYTEA DEFAULT '',
		metadata BYTEA DEFAULT '',
		PRIMARY KEY (pkey, ns)
	)`)
	require.NoError(t, err)
	_, err = first.writeDB.ExecContext(ctx, "INSERT INTO "+table+" (ns, pkey, val, kversion) VALUES ('ns', 'k1', 'v', 'v')")
	require.NoError(t, err)

	second := openSQLiteVault(t, "file:"+path+"?_pragma=busy_timeout(5000)&mode=rw", true)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Go(func() { errs[0] = first.CreateSchema() })
	wg.Go(func() { errs[1] = second.CreateSchema() })
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	requireSQLitePrimaryKey(t, first, "ns,pkey")
	read, err := first.GetState(ctx, "ns", "k1")
	require.NoError(t, err)
	require.NotNil(t, read)
	require.Equal(t, driver.RawValue("v"), read.Raw)
}
