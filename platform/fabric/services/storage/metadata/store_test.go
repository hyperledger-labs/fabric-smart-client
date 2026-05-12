/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type testKey struct{ k string }

func (t testKey) UniqueKey() string { return t.k }

type testMeta struct {
	Name  string
	Value int
}

type mockMetadataStore struct {
	data map[string][]byte
	err  error
}

func (m *mockMetadataStore) GetMetadata(_ context.Context, key string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.data[key], nil
}

func (m *mockMetadataStore) ExistMetadata(_ context.Context, key string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	_, ok := m.data[key]
	return ok, nil
}

func (m *mockMetadataStore) PutMetadata(_ context.Context, key string, data []byte) error {
	if m.err != nil {
		return m.err
	}
	m.data[key] = data
	return nil
}

func newTestStore() *store[testKey, testMeta] {
	return &store[testKey, testMeta]{m: &mockMetadataStore{data: make(map[string][]byte)}}
}

func TestMetadataPutAndGet(t *testing.T) {
	s := newTestStore()
	ctx := context.Background()
	key := testKey{"tx1"}
	meta := testMeta{Name: "alice", Value: 42}

	require.NoError(t, s.PutMetadata(ctx, key, meta))

	got, err := s.GetMetadata(ctx, key)
	require.NoError(t, err)
	require.Equal(t, meta, got)
}

func TestMetadataExistsFalseBeforePut(t *testing.T) {
	s := newTestStore()
	exists, err := s.ExistMetadata(context.Background(), testKey{"missing"})
	require.NoError(t, err)
	require.False(t, exists)
}

func TestMetadataExistsTrueAfterPut(t *testing.T) {
	s := newTestStore()
	ctx := context.Background()
	key := testKey{"tx2"}
	require.NoError(t, s.PutMetadata(ctx, key, testMeta{Name: "bob"}))
	exists, err := s.ExistMetadata(ctx, key)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestMetadataJSONRoundTrip(t *testing.T) {
	s := newTestStore()
	ctx := context.Background()
	key := testKey{"tx3"}
	meta := testMeta{Name: "charlie", Value: 99}

	require.NoError(t, s.PutMetadata(ctx, key, meta))

	// verify raw bytes are valid JSON
	raw := s.m.(*mockMetadataStore).data[key.UniqueKey()]
	var decoded testMeta
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, meta, decoded)
}

func TestMetadataPropagatesErrors(t *testing.T) {
	dbErr := errors.New("db error")
	s := &store[testKey, testMeta]{m: &mockMetadataStore{data: make(map[string][]byte), err: dbErr}}
	ctx := context.Background()
	require.ErrorIs(t, s.PutMetadata(ctx, testKey{"tx1"}, testMeta{}), dbErr)
	_, err := s.GetMetadata(ctx, testKey{"tx1"})
	require.ErrorIs(t, err, dbErr)
	_, err = s.ExistMetadata(ctx, testKey{"tx1"})
	require.ErrorIs(t, err, dbErr)
}
