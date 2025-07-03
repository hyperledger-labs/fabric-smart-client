/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"context"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockKVS struct{}

func (k mockKVS) GetLongTerm(ctx context.Context, ephemeral view.Identity) (view.Identity, error) {
	return nil, nil
}
func (k mockKVS) HaveSameBinding(ctx context.Context, this, that view.Identity) (bool, error) {
	return false, nil
}
func (k mockKVS) PutBinding(ctx context.Context, ephemeral, longTerm view.Identity) error { return nil }

type mockExtractor struct{}

func (m mockExtractor) ExtractPublicKey(id view.Identity) (any, error) {
	return []byte("id"), nil
}

func TestPKIResolveConcurrency(t *testing.T) {
	svc, err := NewService(mockKVS{})
	require.NoError(t, err)

	ext := mockExtractor{}
	resolver := &Resolver{}

	err = svc.AddPublicKeyExtractor(ext)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			svc.pkiResolve(resolver)
		}()
	}
	wg.Wait()
}

func TestGetIdentity(t *testing.T) {
	// setup
	service, err := NewService(mockKVS{})
	require.NoError(t, err)
	ext := mockExtractor{}
	_, err = service.AddResolver("alice", "domain", map[string]string{string(P2PPort): "localhost:1010"}, []string{"apple", "strawberry"}, []byte("alice_id"))
	require.NoError(t, err)
	resolvers := service.Resolvers()
	assert.Len(t, resolvers, 1)
	_, err = service.AddResolver("alice", "domain", map[string]string{string(P2PPort): "localhost:1010"}, []string{"apple", "strawberry"}, []byte("alice_id"))
	require.NoError(t, err)
	resolvers = service.Resolvers()
	assert.Len(t, resolvers, 1)

	err = service.AddPublicKeyExtractor(ext)
	require.NoError(t, err)

	// found
	for _, label := range []string{"alice", "apple", "alice.domain", "strawberry", "localhost:1010", "[::1]:1010", "alice_id"} {
		resultID, err := service.GetIdentity(label, []byte{})
		require.NoError(t, err)
		assert.Equal(t, []byte("alice_id"), []byte(resultID))

		resultID, _, _, err = service.Resolve(context.Background(), view.Identity(label))
		require.NoError(t, err)
		assert.Equal(t, []byte("alice_id"), []byte(resultID))

		resolver, _, err := service.Resolver(context.Background(), view.Identity(label))
		require.NoError(t, err)
		assert.Equal(t, []byte("alice_id"), resolver.ID)

		resolver, err = service.GetResolver(context.Background(), view.Identity(label))
		require.NoError(t, err)
		assert.Equal(t, []byte("alice_id"), resolver.ID)
	}

	// not found
	for _, label := range []string{"pineapple", "bob", "localhost:8080"} {
		resultID, err := service.GetIdentity(label, []byte("no"))
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrNotFound)
		assert.Equal(t, []byte(nil), []byte(resultID))

		_, _, _, err = service.Resolve(context.Background(), view.Identity(label))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotFound)

		_, _, err = service.Resolver(context.Background(), view.Identity(label))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotFound)

		_, err = service.GetResolver(context.Background(), view.Identity(label))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNotFound)
	}
}
