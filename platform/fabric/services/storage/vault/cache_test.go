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

// SetStatuses caches a Valid status, and drops the entry for any other, which another
// replica may still change.
func TestCachedStoreSetStatusesUpdatesCache(t *testing.T) {
	t.Parallel()
	backed := newMockVaultStore()
	s := newCachedTestStore(backed)

	require.NoError(t, s.SetStatuses(context.Background(), driver.Valid, "ok", "tx1", "tx2"))
	for _, txID := range []driver.TxID{"tx1", "tx2"} {
		e, ok := s.cache.Get(txID)
		require.True(t, ok)
		require.Equal(t, driver.Valid, e.Code)
		require.Equal(t, "ok", e.Message)
	}

	s.cache.Put("tx3", &entry{Code: driver.Valid})
	require.NoError(t, s.SetStatuses(context.Background(), driver.Invalid, "bad tx", "tx1", "tx3"))
	for _, txID := range []driver.TxID{"tx1", "tx3"} {
		_, ok := s.cache.Get(txID)
		require.False(t, ok, "an Invalid status is not cached")
		got, err := s.GetTxStatus(t.Context(), txID)
		require.NoError(t, err)
		require.Equal(t, driver.Invalid, got.Code)
		require.Equal(t, "bad tx", got.Message)
	}
}

func TestCachedStoreSetStatusesBusyRereadsBacked(t *testing.T) {
	t.Parallel()
	backed := newMockVaultStore()
	s := newCachedTestStore(backed)
	s.cache.Put("tx1", &entry{Code: driver.Valid})

	require.NoError(t, s.SetStatuses(t.Context(), driver.Busy, "", "tx1"))

	before := backed.getCalls.Load()
	_, err := s.GetTxStatus(t.Context(), "tx1")
	require.NoError(t, err)
	require.Equal(t, before+1, backed.getCalls.Load())
}

// Replicas of a node share the backing store, and another replica may commit a transaction
// at any time, even one this replica discarded, so every status but Valid is reread from the
// backing store on every call. A Valid status is served from the cache.
func TestCachedStoreGetTxStatusCachesOnlyValid(t *testing.T) {
	t.Parallel()
	name := map[driver.TxStatusCode]string{driver.Unknown: "unknown", driver.Valid: "valid", driver.Invalid: "invalid", driver.Busy: "busy"}
	for _, before := range []driver.TxStatusCode{driver.Busy, driver.Unknown, driver.Invalid} {
		t.Run(name[before]+" to valid", func(t *testing.T) {
			t.Parallel()
			backed := newMockVaultStore()
			backed.statuses["tx1"] = &driver.TxStatus{TxID: "tx1", Code: before}
			s := newCachedTestStore(backed)

			got, err := s.GetTxStatus(t.Context(), "tx1")
			require.NoError(t, err)
			require.Equal(t, before, got.Code)
			calls := backed.getCalls.Load()
			_, err = s.GetTxStatus(t.Context(), "tx1")
			require.NoError(t, err)
			require.Equal(t, calls+1, backed.getCalls.Load(), "a status other than Valid is reread")

			// Another replica commits the transaction.
			backed.statuses["tx1"] = &driver.TxStatus{TxID: "tx1", Code: driver.Valid, Message: "committed"}
			got, err = s.GetTxStatus(t.Context(), "tx1")
			require.NoError(t, err)
			require.Equal(t, driver.Valid, got.Code)
			require.Equal(t, "committed", got.Message)

			calls = backed.getCalls.Load()
			got, err = s.GetTxStatus(t.Context(), "tx1")
			require.NoError(t, err)
			require.Equal(t, driver.Valid, got.Code)
			require.Equal(t, calls, backed.getCalls.Load(), "a Valid status is served from the cache")
		})
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
