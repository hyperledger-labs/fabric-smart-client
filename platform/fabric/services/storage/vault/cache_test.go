/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// mockCache implements the cache interface.
type mockCache struct {
	data map[string]*entry
}

func newMockCache() *mockCache { return &mockCache{data: make(map[string]*entry)} }

func (m *mockCache) Get(key string) (*entry, bool) {
	e, ok := m.data[key]
	return e, ok
}
func (m *mockCache) Add(key string, value *entry) { m.data[key] = value }
func (m *mockCache) Delete(key string)            { delete(m.data, key) }

// mockVaultStore implements driver.VaultStore with only GetTxStatus, Store, SetStatuses active.
type mockVaultStore struct {
	statuses map[driver.TxID]*driver.TxStatus
	storeErr error
	setErr   error
}

func newMockVaultStore() *mockVaultStore {
	return &mockVaultStore{statuses: make(map[driver.TxID]*driver.TxStatus)}
}

func (m *mockVaultStore) GetTxStatus(_ context.Context, txID driver.TxID) (*driver.TxStatus, error) {
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
func (m *mockVaultStore) GetStateMetadata(_ context.Context, _ driver.Namespace, _ driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	return nil, nil, nil
}

func (m *mockVaultStore) GetState(_ context.Context, _ driver.Namespace, _ driver.PKey) (*driver.VaultRead, error) {
	return nil, nil
}

func (m *mockVaultStore) GetStates(_ context.Context, _ driver.Namespace, _ ...driver.PKey) (driver.TxStateIterator, error) {
	return nil, nil
}

func (m *mockVaultStore) GetStateRange(_ context.Context, _ driver.Namespace, _, _ driver.PKey) (driver.TxStateIterator, error) {
	return nil, nil
}

func (m *mockVaultStore) GetAllStates(_ context.Context, _ driver.Namespace) (driver.TxStateIterator, error) {
	return nil, nil
}
func (m *mockVaultStore) GetLast(_ context.Context) (*driver.TxStatus, error) { return nil, nil }
func (m *mockVaultStore) GetTxStatuses(_ context.Context, _ ...driver.TxID) (driver.TxStatusIterator, error) {
	return nil, nil
}

func (m *mockVaultStore) GetAllTxStatuses(_ context.Context, _ driver.Pagination) (*driver.PageIterator[*driver.TxStatus], error) {
	return nil, nil
}

func (m *mockVaultStore) NewTxLockVaultReader(_ context.Context, _ driver.TxID, _ driver.IsolationLevel) (driver.LockedVaultReader, error) {
	return nil, nil
}

func (m *mockVaultStore) NewGlobalLockVaultReader(_ context.Context) (driver.LockedVaultReader, error) {
	return nil, nil
}
func (m *mockVaultStore) Close() error { return nil }

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
	return &cachedStore{VaultStore: backed, cache: newMockCache()}
}

func TestCachedStoreGetTxStatusCacheHit(t *testing.T) {
	t.Parallel()
	backed := newMockVaultStore()
	s := newCachedTestStore(backed)
	// Pre-populate cache
	s.cache.Add("tx1", &entry{Code: driver.Valid, Message: "ok"})

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
	s.cache.Add("tx1", &entry{Code: driver.Valid})
	s.cache.Add("tx2", &entry{Code: driver.Valid})

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
