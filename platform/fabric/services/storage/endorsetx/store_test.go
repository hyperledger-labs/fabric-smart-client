/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsetx

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	mem "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

// testKey is a minimal identifier implementation for testing.
type testKey struct{ k string }

func (t testKey) UniqueKey() string { return t.k }

// mockEndorseTxStore is an in-memory implementation of driver2.EndorseTxStore for testing.
type mockEndorseTxStore struct {
	data map[string][]byte
	err  error
}

func (m *mockEndorseTxStore) GetEndorseTx(_ context.Context, key string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.data[key], nil
}

func (m *mockEndorseTxStore) ExistsEndorseTx(_ context.Context, key string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	_, ok := m.data[key]
	return ok, nil
}

func (m *mockEndorseTxStore) PutEndorseTx(_ context.Context, key string, etx []byte) error {
	if m.err != nil {
		return m.err
	}
	m.data[key] = etx
	return nil
}

func newTestStore() *endorseTxStore[testKey] {
	return &endorseTxStore[testKey]{e: &mockEndorseTxStore{data: make(map[string][]byte)}}
}

func TestEndorseTxPutAndGet(t *testing.T) {
	t.Parallel()
	s := newTestStore()
	ctx := context.Background()
	key := testKey{"tx1"}
	payload := []byte("endorsement-data")

	require.NoError(t, s.PutEndorseTx(ctx, key, payload))

	got, err := s.GetEndorseTx(ctx, key)
	require.NoError(t, err)
	require.Equal(t, payload, got)
}

func TestEndorseTxExistsFalseBeforePut(t *testing.T) {
	t.Parallel()
	s := newTestStore()
	exists, err := s.ExistsEndorseTx(context.Background(), testKey{"missing"})
	require.NoError(t, err)
	require.False(t, exists)
}

func TestEndorseTxExistsTrueAfterPut(t *testing.T) {
	t.Parallel()
	s := newTestStore()
	ctx := context.Background()
	key := testKey{"tx2"}
	require.NoError(t, s.PutEndorseTx(ctx, key, []byte("data")))
	exists, err := s.ExistsEndorseTx(ctx, key)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestEndorseTxUniqueKeyRouting(t *testing.T) {
	t.Parallel()
	s := newTestStore()
	ctx := context.Background()
	require.NoError(t, s.PutEndorseTx(ctx, testKey{"a"}, []byte("aaa")))
	require.NoError(t, s.PutEndorseTx(ctx, testKey{"b"}, []byte("bbb")))
	gotA, err := s.GetEndorseTx(ctx, testKey{"a"})
	require.NoError(t, err)
	require.Equal(t, []byte("aaa"), gotA)
	gotB, err := s.GetEndorseTx(ctx, testKey{"b"})
	require.NoError(t, err)
	require.Equal(t, []byte("bbb"), gotB)
}

func TestEndorseTxPropagatesErrors(t *testing.T) {
	t.Parallel()
	dbErr := errors.New("db error")
	s := &endorseTxStore[testKey]{e: &mockEndorseTxStore{data: make(map[string][]byte), err: dbErr}}
	ctx := context.Background()
	require.ErrorIs(t, s.PutEndorseTx(ctx, testKey{"tx1"}, []byte("data")), dbErr)
	_, err := s.GetEndorseTx(ctx, testKey{"tx1"})
	require.ErrorIs(t, err, dbErr)
	_, err = s.ExistsEndorseTx(ctx, testKey{"tx1"})
	require.ErrorIs(t, err, dbErr)
}

func TestNewStoreMemory(t *testing.T) {
	t.Parallel()
	cp := multiplexed.MockTypeConfig(mem.Persistence, struct{}{})
	d := multiplexed.NewDriver(cp, mem.NewNamedDriver(sqlite2.NewDbProvider()))
	store, err := NewStore[testKey](cp, d)
	require.NoError(t, err)
	require.NotNil(t, store)
}
