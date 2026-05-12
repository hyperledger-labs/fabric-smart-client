/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package envelope

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type testKey struct{ k string }

func (t testKey) UniqueKey() string { return t.k }

type mockEnvelopeStore struct {
	data map[string][]byte
	err  error
}

func (m *mockEnvelopeStore) GetEnvelope(_ context.Context, key string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.data[key], nil
}

func (m *mockEnvelopeStore) ExistsEnvelope(_ context.Context, key string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	_, ok := m.data[key]
	return ok, nil
}

func (m *mockEnvelopeStore) PutEnvelope(_ context.Context, key string, etx []byte) error {
	if m.err != nil {
		return m.err
	}
	m.data[key] = etx
	return nil
}

func newTestStore() *envelopeStore[testKey] {
	return &envelopeStore[testKey]{e: &mockEnvelopeStore{data: make(map[string][]byte)}}
}

func TestEnvelopePutAndGet(t *testing.T) {
	s := newTestStore()
	ctx := context.Background()
	key := testKey{"tx1"}
	payload := []byte("envelope-bytes")
	require.NoError(t, s.PutEnvelope(ctx, key, payload))
	got, err := s.GetEnvelope(ctx, key)
	require.NoError(t, err)
	require.Equal(t, payload, got)
}

func TestEnvelopeExistsFalseBeforePut(t *testing.T) {
	s := newTestStore()
	exists, err := s.ExistsEnvelope(context.Background(), testKey{"missing"})
	require.NoError(t, err)
	require.False(t, exists)
}

func TestEnvelopeExistsTrueAfterPut(t *testing.T) {
	s := newTestStore()
	ctx := context.Background()
	key := testKey{"tx2"}
	require.NoError(t, s.PutEnvelope(ctx, key, []byte("data")))
	exists, err := s.ExistsEnvelope(ctx, key)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestEnvelopeUniqueKeyRouting(t *testing.T) {
	s := newTestStore()
	ctx := context.Background()
	require.NoError(t, s.PutEnvelope(ctx, testKey{"a"}, []byte("aaa")))
	require.NoError(t, s.PutEnvelope(ctx, testKey{"b"}, []byte("bbb")))
	gotA, err := s.GetEnvelope(ctx, testKey{"a"})
	require.NoError(t, err)
	require.Equal(t, []byte("aaa"), gotA)
	gotB, err := s.GetEnvelope(ctx, testKey{"b"})
	require.NoError(t, err)
	require.Equal(t, []byte("bbb"), gotB)
}

func TestEnvelopePropagatesErrors(t *testing.T) {
	dbErr := errors.New("db error")
	s := &envelopeStore[testKey]{e: &mockEnvelopeStore{data: make(map[string][]byte), err: dbErr}}
	ctx := context.Background()
	require.ErrorIs(t, s.PutEnvelope(ctx, testKey{"tx1"}, []byte("data")), dbErr)
	_, err := s.GetEnvelope(ctx, testKey{"tx1"})
	require.ErrorIs(t, err, dbErr)
	_, err = s.ExistsEnvelope(ctx, testKey{"tx1"})
	require.ErrorIs(t, err, dbErr)
}
