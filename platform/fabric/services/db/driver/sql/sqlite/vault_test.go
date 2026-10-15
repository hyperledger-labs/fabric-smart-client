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

// TestVaultStoreCreatesNamespaceKeyIndex checks that CreateSchema adds an index
// whose first column is the namespace, so a range scan of one namespace does not
// have to walk every namespace that shares the key range.
func TestVaultStoreCreatesNamespaceKeyIndex(t *testing.T) {
	t.Parallel()

	cp := sqlite2.NewConfigProvider(testing2.MockConfig(sqlite2.Config{
		DataSource: "file:" + filepath.Join(t.TempDir(), "vault.db"),
	}))
	store, err := NewPersistenceWithOpts(cp, sqlite2.NewDbProvider(), "", NewVaultStore)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	tx, err := store.writeDB.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	var name string
	err = tx.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name LIKE 'ns_pkey_%'`).Scan(&name)
	require.NoError(t, err)
	require.NotEmpty(t, name)
}
