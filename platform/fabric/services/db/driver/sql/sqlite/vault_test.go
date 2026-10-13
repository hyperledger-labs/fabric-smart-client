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
	ctx := t.Context()
	require.NoError(t, store.Store(ctx, nil, driver.Writes{"ns": {
		"k3":   {Raw: []byte("k3"), Version: []byte("v")},
		"k2":   {Raw: []byte("k2"), Version: []byte("v")},
		"k111": {Raw: []byte("k111"), Version: []byte("v")},
		"k1":   {Raw: []byte("k1"), Version: []byte("v")},
	}}, nil))

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
