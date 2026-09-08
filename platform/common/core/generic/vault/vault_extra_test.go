/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
)

// newMemoryVault builds a vault backed by an in-memory store, closed when the
// test finishes.
func newMemoryVault(t *testing.T) (*Vault[ValidationCode], driver.VaultStore) {
	t.Helper()

	ddb, err := vault2.OpenMemoryVault(t.Name())
	require.NoError(t, err)
	require.NotNil(t, ddb)
	t.Cleanup(func() { utils.IgnoreErrorFunc(ddb.Close) })

	provider := &testArtifactProvider{
		removeNils: func(items []driver2.VaultRead) []driver2.VaultRead { return items },
	}
	v, err := provider.NewNonCachedVault(ddb)
	require.NoError(t, err)

	return v, ddb
}

// TestVaultRWSExists reports whether an interceptor is currently registered
// for a transaction.
func TestVaultRWSExists(t *testing.T) { //nolint:paralleltest
	v, _ := newMemoryVault(t)
	ctx := t.Context()

	require.False(t, v.RWSExists(ctx, "tx1"), "no interceptor registered yet")

	_, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	require.True(t, v.RWSExists(ctx, "tx1"))

	require.False(t, v.RWSExists(ctx, "other"), "unrelated transactions are unaffected")

	// Unmapping is refused while the interceptor is still open.
	_, err = v.UnmapInterceptor("tx1")
	require.ErrorContains(t, err, "done has not been called")
	require.True(t, v.RWSExists(ctx, "tx1"))

	rws, err := v.NewRWSet(ctx, "tx2")
	require.NoError(t, err)
	rws.Done()

	unmapped, err := v.UnmapInterceptor("tx2")
	require.NoError(t, err)
	require.NotNil(t, unmapped)
	require.False(t, v.RWSExists(ctx, "tx2"), "unmapping a closed interceptor removes it")

	_, err = v.UnmapInterceptor("absent")
	require.ErrorContains(t, err, "could not be found")
}

// TestVaultNewRWSetWithIsolationLevel checks the explicit-isolation entry point
// registers an interceptor the same way NewRWSet does.
func TestVaultNewRWSetWithIsolationLevel(t *testing.T) { //nolint:paralleltest
	v, _ := newMemoryVault(t)
	ctx := t.Context()

	rws, err := v.NewRWSetWithIsolationLevel(ctx, "tx1", driver2.LevelDefault)
	require.NoError(t, err)
	require.NotNil(t, rws)
	require.True(t, v.RWSExists(ctx, "tx1"))

	_, err = v.NewRWSetWithIsolationLevel(ctx, "tx1", driver2.LevelDefault)
	require.Error(t, err, "a second rwset for the same transaction is rejected")
}

// TestVaultSetStatusAndStatuses round-trips validation codes through the store.
func TestVaultSetStatusAndStatuses(t *testing.T) { //nolint:paralleltest
	v, _ := newMemoryVault(t)
	ctx := t.Context()

	require.NoError(t, v.SetStatus(ctx, "tx1", valid))
	require.NoError(t, v.SetStatus(ctx, "tx2", invalid))

	code, _, err := v.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, valid, code)

	statuses, err := v.Statuses(ctx, "tx1", "tx2")
	require.NoError(t, err)
	require.Len(t, statuses, 2)

	byTxID := map[driver2.TxID]driver2.TxValidationStatus[ValidationCode]{}
	for _, s := range statuses {
		byTxID[s.TxID] = s
	}
	require.Equal(t, valid, byTxID["tx1"].ValidationCode)
	require.Equal(t, invalid, byTxID["tx2"].ValidationCode)
}

// TestVaultStatusesEmpty checks querying no transactions is not an error.
func TestVaultStatusesEmpty(t *testing.T) { //nolint:paralleltest
	v, _ := newMemoryVault(t)

	statuses, err := v.Statuses(t.Context())
	require.NoError(t, err)
	require.Empty(t, statuses)
}

// TestVaultSetDiscarded marks a transaction invalid and records the reason.
func TestVaultSetDiscarded(t *testing.T) { //nolint:paralleltest
	v, _ := newMemoryVault(t)
	ctx := t.Context()

	require.NoError(t, v.SetDiscarded(ctx, "tx1", "conflicting write"))

	code, message, err := v.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, invalid, code)
	require.Equal(t, "conflicting write", message)

	statuses, err := v.Statuses(ctx, "tx1")
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.Equal(t, "conflicting write", statuses[0].Message)
}

// TestVaultClose closes the underlying store.
func TestVaultClose(t *testing.T) { //nolint:paralleltest
	ddb, err := vault2.OpenMemoryVault(t.Name())
	require.NoError(t, err)

	provider := &testArtifactProvider{
		removeNils: func(items []driver2.VaultRead) []driver2.VaultRead { return items },
	}
	v, err := provider.NewNonCachedVault(ddb)
	require.NoError(t, err)

	require.NoError(t, v.Close())
}

var _ = context.Background
