/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestPKIResolveConcurrency(t *testing.T) {
	t.Parallel()
	bindingStore := &mock.BindingStore{}
	bindingStore.GetLongTermReturns(nil, nil)
	bindingStore.HaveSameBindingReturns(false, nil)
	bindingStore.PutBindingsReturns(nil)

	svc, err := endpoint.NewService(bindingStore)
	require.NoError(t, err)

	extractor := &mock.PublicKeyExtractor{}
	extractor.ExtractPublicKeyReturns([]byte("id"), nil)
	resolver := &endpoint.Resolver{}

	err = svc.AddPublicKeyExtractor(extractor)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			svc.PkiResolve(resolver)
		}()
	}
	wg.Wait()
}

func TestGetIdentity(t *testing.T) {
	t.Parallel()
	// setup
	bindingStore := &mock.BindingStore{}
	bindingStore.PutBindingsReturns(nil)

	service, err := endpoint.NewService(bindingStore)
	require.NoError(t, err)

	extractor := &mock.PublicKeyExtractor{}
	extractor.ExtractPublicKeyReturns([]byte("id"), nil)

	_, err = service.AddResolver(
		"alice",
		"fsc.domain",
		map[string]string{string(endpoint.P2PPort): "localhost:1010"},
		[]string{"apple", "strawberry"},
		[]byte("alice_id"),
	)
	require.NoError(t, err)
	resolvers := service.Resolvers()
	require.Len(t, resolvers, 1)
	require.Equal(t, 0, bindingStore.PutBindingsCallCount())

	_, err = service.AddResolver(
		"alice",
		"fabric.domain",
		map[string]string{string(endpoint.P2PPort): "localhost:1010"},
		[]string{"apricot"},
		[]byte("alice_id2"),
	)
	require.NoError(t, err)
	resolvers = service.Resolvers()
	require.Len(t, resolvers, 1)
	require.Equal(t, 1, bindingStore.PutBindingsCallCount())

	err = service.AddPublicKeyExtractor(extractor)
	require.NoError(t, err)

	// found
	for _, label := range []string{
		"alice",
		"alice.fsc.domain",
		"apple",
		"strawberry",
		"localhost:1010",
		"alice_id",
	} {
		resultID, err := service.GetIdentity(label, []byte{})
		require.NoError(t, err)
		require.Equal(t, []byte("alice_id"), []byte(resultID))

		resultID, _, _, err = service.Resolve(t.Context(), view.Identity(label))
		require.NoError(t, err)
		require.Equal(t, []byte("alice_id"), []byte(resultID))

		resolver, _, err := service.Resolver(t.Context(), view.Identity(label))
		require.NoError(t, err)
		require.Equal(t, []byte("alice_id"), resolver.ID)

		resolver, err = service.GetResolver(t.Context(), view.Identity(label))
		require.NoError(t, err)
		require.Equal(t, []byte("alice_id"), resolver.ID)
	}

	// not found
	for _, label := range []string{
		"alice.fabric.domain",
		"pineapple",
		"bob",
		"apricot",
		"localhost:8080",
		"alice_id2",
	} {
		resultID, err := service.GetIdentity(label, []byte("no"))
		require.Error(t, err)
		require.ErrorIs(t, err, endpoint.ErrNotFound)
		require.Equal(t, []byte(nil), []byte(resultID))

		_, _, _, err = service.Resolve(t.Context(), view.Identity(label))
		require.Error(t, err)
		require.ErrorIs(t, err, endpoint.ErrNotFound)

		_, _, err = service.Resolver(t.Context(), view.Identity(label))
		require.Error(t, err)
		require.ErrorIs(t, err, endpoint.ErrNotFound)

		_, err = service.GetResolver(t.Context(), view.Identity(label))
		require.Error(t, err)
		require.ErrorIs(t, err, endpoint.ErrNotFound)
	}

	for _, label := range []string{
		"alice",
		"alice.fsc.domain",
		"apple",
		"strawberry",
		"localhost:1010",
		"alice_id",
	} {
		ok, err := service.RemoveResolver(view.Identity(label))
		require.NoError(t, err, "failed to remove resolver for [%s]", label)
		require.True(t, ok)

		resolvers = service.Resolvers()
		require.Empty(t, resolvers)

		// add again
		_, err = service.AddResolver(
			"alice",
			"fsc.domain",
			map[string]string{string(endpoint.P2PPort): "localhost:1010"},
			[]string{"apple", "strawberry"},
			[]byte("alice_id"),
		)
		require.NoError(t, err)
	}

	// remove something that does not exist
	ok, err := service.RemoveResolver(view.Identity("bob"))
	require.Error(t, err)
	require.ErrorIs(t, err, endpoint.ErrNotFound)
	require.False(t, ok)
}
