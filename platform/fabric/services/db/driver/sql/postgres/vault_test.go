/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// newPostgresVaultStore starts a postgres container and returns a VaultStore
// whose schema has been created against it. The container is terminated through
// testing.Cleanup.
//
// It goes through NewPersistenceWithOpts rather than buildVaultStore so that
// CreateSchema runs the same way it does in production.
func newPostgresVaultStore(tb testing.TB) *VaultStore {
	tb.Helper()

	terminate, pgConnStr, err := postgres2.StartPostgres(tb.Context(), postgres2.ConfigFromEnv(), nil)
	require.NoError(tb, err)
	tb.Cleanup(terminate)

	cp := postgres2.NewConfigProvider(testing2.MockConfig(postgres2.Config{
		DataSource: pgConnStr,
	}))
	store, err := NewPersistenceWithOpts(cp, postgres2.NewDbProvider(), "", NewVaultStore)
	require.NoError(tb, err)

	return store
}

// TestPostgresVaultStore covers Store: the statuses it marks busy then valid,
// and the states and metadata it upserts, all inside one transaction. The name
// carries "Postgres" so CI's --run/--skip gate gives it to the container suite
// only.
func TestPostgresVaultStore(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()

	txIDs := []driver.TxID{"tx1", "tx2"}
	writes := driver.Writes{
		"ns1": {
			"k1": driver.VaultValue{Raw: []byte("v1"), Version: []byte("ver1")},
			"k2": driver.VaultValue{Raw: []byte("v2"), Version: []byte("ver2")},
		},
	}
	metaWrites := driver.MetaWrites{
		"ns1": {
			"k1": driver.VaultMetadataValue{
				Version:  []byte("ver1"),
				Metadata: map[string][]byte{"m": []byte("meta1")},
			},
		},
	}

	require.NoError(t, store.Store(ctx, txIDs, writes, metaWrites))

	r, err := store.NewGlobalLockVaultReader(ctx)
	require.NoError(t, err)

	// Store leaves every tx it was given valid, not busy.
	for _, txID := range txIDs {
		status, err := r.GetTxStatus(ctx, txID)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.Equal(t, driver.Valid, status.Code)
	}

	state, err := r.GetState(ctx, "ns1", "k1")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, driver.RawValue("v1"), state.Raw)
	require.Equal(t, driver.RawVersion("ver1"), state.Version)

	state, err = r.GetState(ctx, "ns1", "k2")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, driver.RawValue("v2"), state.Raw)

	meta, version, err := r.GetStateMetadata(ctx, "ns1", "k1")
	require.NoError(t, err)
	require.Equal(t, driver.RawVersion("ver1"), version)
	require.Equal(t, []byte("meta1"), meta["m"])
	require.NoError(t, r.Done())
}

// Store is called with no statuses and no writes on transactions that change
// nothing, so the empty case must commit cleanly rather than fail on an empty
// query.
func TestPostgresVaultStoreEmpty(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	require.NoError(t, store.Store(t.Context(), nil, nil, nil))
}

// Statuses and states are written independently: a call carrying only txIDs
// records them without any state, and a call carrying only writes records
// state without touching statuses.
func TestPostgresVaultStoreStatusesOnly(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()

	require.NoError(t, store.Store(ctx, []driver.TxID{"txOnly"}, nil, nil))

	r, err := store.NewGlobalLockVaultReader(ctx)
	require.NoError(t, err)
	status, err := r.GetTxStatus(ctx, "txOnly")
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, driver.Valid, status.Code)
	require.NoError(t, r.Done())
}

func TestPostgresVaultStoreWritesOnly(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()

	require.NoError(t, store.Store(ctx, nil, driver.Writes{
		"ns2": {"k9": driver.VaultValue{Raw: []byte("v9"), Version: []byte("ver9")}},
	}, nil))

	r, err := store.NewGlobalLockVaultReader(ctx)
	require.NoError(t, err)
	state, err := r.GetState(ctx, "ns2", "k9")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, driver.RawValue("v9"), state.Raw)
	require.NoError(t, r.Done())
}

// Store upserts: a second write to the same key replaces the value rather than
// failing on the states table's (pkey, ns) primary key.
func TestPostgresVaultStoreUpsert(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()

	write := func(raw, version string) error {
		return store.Store(ctx, nil, driver.Writes{
			"ns3": {"k1": driver.VaultValue{Raw: []byte(raw), Version: []byte(version)}},
		}, nil)
	}
	require.NoError(t, write("first", "ver1"))
	require.NoError(t, write("second", "ver2"))

	r, err := store.NewGlobalLockVaultReader(ctx)
	require.NoError(t, err)
	state, err := r.GetState(ctx, "ns3", "k1")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, driver.RawValue("second"), state.Raw)
	require.Equal(t, driver.RawVersion("ver2"), state.Version)
	require.NoError(t, r.Done())
}

// CreateSchema is idempotent — every constructor runs it unless
// SkipCreateTable is set, so a second node opening the same database must not
// fail on the existing tables.
func TestPostgresVaultStoreCreateSchemaIdempotent(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	require.NoError(t, store.CreateSchema())
	require.NoError(t, store.CreateSchema())
}
