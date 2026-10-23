/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
)

// mockVaultStore implements driver.VaultStore with only GetTxStatus, Store, SetStatuses active.
type mockVaultStore struct {
	statuses map[driver.TxID]*driver.TxStatus
	storeErr error
	setErr   error
	// getCalls counts GetTxStatus calls that reached the backing store, which
	// is how the benchmarks derive a cache hit rate. Atomic because the
	// parallel benchmark reads through this mock from many goroutines.
	getCalls atomic.Int64
}

func newMockVaultStore() *mockVaultStore {
	return &mockVaultStore{statuses: make(map[driver.TxID]*driver.TxStatus)}
}

func (m *mockVaultStore) GetTxStatus(_ context.Context, txID driver.TxID) (*driver.TxStatus, error) {
	m.getCalls.Add(1)

	s, ok := m.statuses[txID]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (m *mockVaultStore) Store(_ context.Context, txIDs []driver.TxID, _ driver.Writes, _ driver.MetaWrites) error {
	if m.storeErr != nil {
		return m.storeErr
	}
	for _, txID := range txIDs {
		m.statuses[txID] = &driver.TxStatus{TxID: txID, Code: driver.Valid}
	}
	return nil
}

func (m *mockVaultStore) SetStatuses(_ context.Context, code driver.TxStatusCode, message string, txIDs ...driver.TxID) error {
	if m.setErr != nil {
		return m.setErr
	}
	for _, txID := range txIDs {
		m.statuses[txID] = &driver.TxStatus{TxID: txID, Code: code, Message: message}
	}
	return nil
}

// Stub implementations to satisfy driver.VaultStore (embeds VaultReader).
func (*mockVaultStore) GetStateMetadata(_ context.Context, _ driver.Namespace, _ driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	return nil, nil, nil
}

func (*mockVaultStore) GetState(_ context.Context, _ driver.Namespace, _ driver.PKey) (*driver.VaultRead, error) {
	return nil, nil
}

func (*mockVaultStore) GetStates(_ context.Context, _ driver.Namespace, _ ...driver.PKey) (driver.TxStateIterator, error) {
	return nil, nil
}

func (*mockVaultStore) GetStateRange(_ context.Context, _ driver.Namespace, _, _ driver.PKey) (driver.TxStateIterator, error) {
	return nil, nil
}

func (*mockVaultStore) GetAllStates(_ context.Context, _ driver.Namespace) (driver.TxStateIterator, error) {
	return nil, nil
}
func (*mockVaultStore) GetLast(_ context.Context) (*driver.TxStatus, error) { return nil, nil }
func (*mockVaultStore) GetTxStatuses(_ context.Context, _ ...driver.TxID) (driver.TxStatusIterator, error) {
	return nil, nil
}

func (*mockVaultStore) GetAllTxStatuses(_ context.Context, _ driver.Pagination) (*driver.PageIterator[*driver.TxStatus], error) {
	return nil, nil
}

func (*mockVaultStore) NewTxLockVaultReader(_ context.Context, _ driver.TxID, _ driver.IsolationLevel) (driver.LockedVaultReader, error) {
	return nil, nil
}

func (*mockVaultStore) NewGlobalLockVaultReader(_ context.Context) (driver.LockedVaultReader, error) {
	return nil, nil
}
func (*mockVaultStore) Close() error { return nil }

// --- notCachedStore tests ---

func TestNotCachedStoreInvalidateIsNoop(t *testing.T) {
	t.Parallel()
	s := &notCachedStore{VaultStore: newMockVaultStore()}
	// Should not panic and is a no-op
	s.Invalidate("tx1", "tx2")
}

func TestNotCachedStoreDelegatesToBacked(t *testing.T) {
	t.Parallel()
	backed := newMockVaultStore()
	backed.statuses["tx1"] = &driver.TxStatus{TxID: "tx1", Code: driver.Valid}
	s := &notCachedStore{VaultStore: backed}

	got, err := s.GetTxStatus(context.Background(), "tx1")
	require.NoError(t, err)
	require.Equal(t, driver.Valid, got.Code)
}

// --- cachedStore tests ---

func newCachedTestStore(backed *mockVaultStore) *cachedStore {
	return &cachedStore{VaultStore: backed, cache: cache.NewMapCache[driver.TxID, *entry]()}
}

func TestCachedStoreGetTxStatusCacheHit(t *testing.T) {
	t.Parallel()
	backed := newMockVaultStore()
	s := newCachedTestStore(backed)
	// Pre-populate cache
	s.cache.Put("tx1", &entry{Code: driver.Valid, Message: "ok"})

	got, err := s.GetTxStatus(context.Background(), "tx1")
	require.NoError(t, err)
	require.Equal(t, driver.Valid, got.Code)
	require.Equal(t, "ok", got.Message)
	// backed should NOT have been called
	require.Empty(t, backed.statuses)
}

func TestCachedStoreGetTxStatusCacheMissPopulatesCache(t *testing.T) {
	t.Parallel()
	backed := newMockVaultStore()
	backed.statuses["tx1"] = &driver.TxStatus{TxID: "tx1", Code: driver.Valid, Message: "from-backed"}
	s := newCachedTestStore(backed)

	got, err := s.GetTxStatus(context.Background(), "tx1")
	require.NoError(t, err)
	require.Equal(t, driver.Valid, got.Code)

	// Cache should now have the entry
	cached, ok := s.cache.Get("tx1")
	require.True(t, ok)
	require.Equal(t, driver.Valid, cached.Code)
}

func TestCachedStoreGetTxStatusMissNotFound(t *testing.T) {
	t.Parallel()
	s := newCachedTestStore(newMockVaultStore())

	got, err := s.GetTxStatus(context.Background(), "missing")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestCachedStoreInvalidateDeletesFromCache(t *testing.T) {
	t.Parallel()
	s := newCachedTestStore(newMockVaultStore())
	s.cache.Put("tx1", &entry{Code: driver.Valid})
	s.cache.Put("tx2", &entry{Code: driver.Valid})

	s.Invalidate("tx1")

	_, ok := s.cache.Get("tx1")
	require.False(t, ok)
	_, ok = s.cache.Get("tx2")
	require.True(t, ok)
}

func TestCachedStoreStoreUpdatesCache(t *testing.T) {
	t.Parallel()
	backed := newMockVaultStore()
	s := newCachedTestStore(backed)

	err := s.Store(context.Background(), []driver.TxID{"tx1", "tx2"}, nil, nil)
	require.NoError(t, err)

	for _, txID := range []driver.TxID{"tx1", "tx2"} {
		e, ok := s.cache.Get(txID)
		require.True(t, ok)
		require.Equal(t, driver.Valid, e.Code)
	}
}

func TestCachedStoreStorePropagatesError(t *testing.T) {
	t.Parallel()
	backed := newMockVaultStore()
	backed.storeErr = errors.New("store error")
	s := newCachedTestStore(backed)

	err := s.Store(context.Background(), []driver.TxID{"tx1"}, nil, nil)
	require.ErrorIs(t, err, backed.storeErr)

	_, ok := s.cache.Get("tx1")
	require.False(t, ok)
}

func TestCachedStoreSetStatusesUpdatesCache(t *testing.T) {
	t.Parallel()
	backed := newMockVaultStore()
	s := newCachedTestStore(backed)

	err := s.SetStatuses(context.Background(), driver.Invalid, "bad tx", "tx1", "tx2")
	require.NoError(t, err)

	for _, txID := range []driver.TxID{"tx1", "tx2"} {
		e, ok := s.cache.Get(txID)
		require.True(t, ok)
		require.Equal(t, driver.Invalid, e.Code)
		require.Equal(t, "bad tx", e.Message)
	}
}

func TestCachedStoreSetStatusesPropagatesError(t *testing.T) {
	t.Parallel()
	backed := newMockVaultStore()
	backed.setErr = errors.New("set error")
	s := newCachedTestStore(backed)

	err := s.SetStatuses(context.Background(), driver.Invalid, "bad", "tx1")
	require.ErrorIs(t, err, backed.setErr)

	_, ok := s.cache.Get("tx1")
	require.False(t, ok)
}

// TestCachedStoreInvalidateFallsThroughToBacked asserts that a read after
// Invalidate misses the cache and is served from the backing store.
func TestCachedStoreInvalidateFallsThroughToBacked(t *testing.T) {
	t.Parallel()
	backed := newMockVaultStore()
	backed.statuses["tx1"] = &driver.TxStatus{TxID: "tx1", Code: driver.Valid, Message: "from backed"}

	s := &cachedStore{
		VaultStore: backed,
		cache:      cache.NewSecondChanceCache[driver.TxID, *entry](10, 1, nil),
	}

	s.cache.Put("tx1", &entry{Code: driver.Busy, Message: "stale"})
	s.Invalidate("tx1")

	got, err := s.GetTxStatus(context.Background(), "tx1")
	require.NoError(t, err)
	require.Equal(t, driver.Valid, got.Code)
	require.Equal(t, "from backed", got.Message)
}
