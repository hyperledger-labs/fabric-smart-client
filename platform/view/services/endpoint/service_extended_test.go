/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package endpoint_test contains comprehensive tests for the endpoint service,
// including tests for binding, resolver management, PKI extraction, and concurrency.
package endpoint_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestBind(t *testing.T) {
	t.Parallel()
	t.Run("bind single ephemeral identity", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.PutBindingsReturns(nil)

		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		longTerm := view.Identity("long-term-id")
		ephemeral := view.Identity("ephemeral-id")

		err = service.Bind(context.Background(), longTerm, ephemeral)
		require.NoError(t, err)

		assert.Equal(t, 1, bindingStore.PutBindingsCallCount())
		ctx, lt, ephs := bindingStore.PutBindingsArgsForCall(0)
		assert.NotNil(t, ctx)
		assert.Equal(t, longTerm, lt)
		assert.Equal(t, []view.Identity{ephemeral}, ephs)
	})

	t.Run("bind multiple ephemeral identities", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.PutBindingsReturns(nil)

		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		longTerm := view.Identity("long-term-id")
		ephemeral1 := view.Identity("ephemeral-id-1")
		ephemeral2 := view.Identity("ephemeral-id-2")
		ephemeral3 := view.Identity("ephemeral-id-3")

		err = service.Bind(context.Background(), longTerm, ephemeral1, ephemeral2, ephemeral3)
		require.NoError(t, err)

		assert.Equal(t, 1, bindingStore.PutBindingsCallCount())
		_, _, ephs := bindingStore.PutBindingsArgsForCall(0)
		assert.Equal(t, []view.Identity{ephemeral1, ephemeral2, ephemeral3}, ephs)
	})

	t.Run("filter out identities equal to longTerm", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.PutBindingsReturns(nil)

		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		longTerm := view.Identity("long-term-id")
		ephemeral1 := view.Identity("ephemeral-id-1")
		ephemeral2 := view.Identity("long-term-id") // Same as longTerm
		ephemeral3 := view.Identity("ephemeral-id-3")

		err = service.Bind(context.Background(), longTerm, ephemeral1, ephemeral2, ephemeral3)
		require.NoError(t, err)

		assert.Equal(t, 1, bindingStore.PutBindingsCallCount())
		_, _, ephs := bindingStore.PutBindingsArgsForCall(0)
		// ephemeral2 should be filtered out
		assert.Equal(t, []view.Identity{ephemeral1, ephemeral3}, ephs)
	})

	t.Run("no binding when all ephemeral IDs equal longTerm", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.PutBindingsReturns(nil)

		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		longTerm := view.Identity("long-term-id")

		err = service.Bind(context.Background(), longTerm, longTerm, longTerm)
		require.NoError(t, err)

		// Should not call PutBindings since all IDs are filtered out
		assert.Equal(t, 0, bindingStore.PutBindingsCallCount())
	})

	t.Run("no binding when no ephemeral IDs provided", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.PutBindingsReturns(nil)

		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		longTerm := view.Identity("long-term-id")

		err = service.Bind(context.Background(), longTerm)
		require.NoError(t, err)

		assert.Equal(t, 0, bindingStore.PutBindingsCallCount())
	})

	t.Run("error from binding store", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		expectedErr := errors.New("storage error")
		bindingStore.PutBindingsReturns(expectedErr)

		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		longTerm := view.Identity("long-term-id")
		ephemeral := view.Identity("ephemeral-id")

		err = service.Bind(context.Background(), longTerm, ephemeral)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed storing bindings")
	})
}

func TestIsBoundTo(t *testing.T) {
	t.Parallel()
	t.Run("identities are bound", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.HaveSameBindingReturns(true, nil)

		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id1 := view.Identity("id1")
		id2 := view.Identity("id2")

		result := service.IsBoundTo(context.Background(), id1, id2)
		assert.True(t, result)

		assert.Equal(t, 1, bindingStore.HaveSameBindingCallCount())
		ctx, a, b := bindingStore.HaveSameBindingArgsForCall(0)
		assert.NotNil(t, ctx)
		assert.Equal(t, id1, a)
		assert.Equal(t, id2, b)
	})

	t.Run("identities are not bound", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.HaveSameBindingReturns(false, nil)

		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id1 := view.Identity("id1")
		id2 := view.Identity("id2")

		result := service.IsBoundTo(context.Background(), id1, id2)
		assert.False(t, result)
	})

	t.Run("error from binding store returns false", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.HaveSameBindingReturns(false, errors.New("storage error"))

		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id1 := view.Identity("id1")
		id2 := view.Identity("id2")

		result := service.IsBoundTo(context.Background(), id1, id2)
		assert.False(t, result)
	})
}

func TestUpdateResolver(t *testing.T) {
	t.Parallel()
	t.Run("update existing resolver addresses", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		// Add initial resolver
		id := []byte("alice-id")
		_, err = service.AddResolver(
			"alice",
			"example.com",
			map[string]string{"p2p": "localhost:8080"},
			[]string{"alice-alias"},
			id,
		)
		require.NoError(t, err)

		// Update with new addresses
		newID, err := service.UpdateResolver(
			"alice",
			"example.com",
			map[string]string{"p2p": "localhost:9090", "view": "localhost:9091"},
			[]string{"alice-alias", "new-alias"},
			id,
		)
		require.NoError(t, err)
		assert.Equal(t, view.Identity(id), newID)

		// Verify the resolver was updated
		resolver, err := service.GetResolver(context.Background(), view.Identity(id))
		require.NoError(t, err)
		// localhost resolves to either IPv4 or IPv6
		assert.Contains(t, []string{"localhost:9090", "127.0.0.1:9090", "[::1]:9090"}, resolver.Addresses[endpoint.P2PPort])
		assert.Contains(t, []string{"localhost:9091", "127.0.0.1:9091", "[::1]:9091"}, resolver.Addresses[endpoint.ViewPort])
	})

	t.Run("update adds new aliases", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id := []byte("bob-id")
		_, err = service.AddResolver(
			"bob",
			"example.com",
			map[string]string{"p2p": "localhost:8080"},
			[]string{"bob-alias"},
			id,
		)
		require.NoError(t, err)

		// Update with new alias
		_, err = service.UpdateResolver(
			"bob",
			"example.com",
			map[string]string{"p2p": "localhost:8080"},
			[]string{"bob-alias", "bob-new-alias"},
			id,
		)
		require.NoError(t, err)

		// Verify new alias works
		resolver, err := service.GetResolver(context.Background(), view.Identity("bob-new-alias"))
		require.NoError(t, err)
		assert.Equal(t, id, resolver.ID)
	})

	t.Run("update non-existent resolver adds it", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.PutBindingsReturns(nil)
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id := []byte("charlie-id")
		newID, err := service.UpdateResolver(
			"charlie",
			"example.com",
			map[string]string{"p2p": "localhost:8080"},
			[]string{"charlie-alias"},
			id,
		)
		require.NoError(t, err)
		assert.Nil(t, newID)

		// Verify the resolver was added
		resolver, err := service.GetResolver(context.Background(), view.Identity(id))
		require.NoError(t, err)
		assert.Equal(t, "charlie", resolver.Name)
	})
}

func TestSetPublicKeyIDSynthesizer(t *testing.T) {
	t.Parallel()
	t.Run("set custom synthesizer", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		// Add a public key extractor that returns a valid key
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		extractor := &mockPublicKeyExtractor{
			key: &privateKey.PublicKey,
		}
		err = service.AddPublicKeyExtractor(extractor)
		require.NoError(t, err)

		// Create a custom synthesizer
		customSynthesizer := &mockPublicKeyIDSynthesizer{
			id: []byte("custom-id"),
		}

		service.SetPublicKeyIDSynthesizer(customSynthesizer)

		// Add a resolver and verify custom synthesizer is used
		id := []byte("test-id")
		_, err = service.AddResolver(
			"test",
			"example.com",
			map[string]string{"p2p": "localhost:8080"},
			nil,
			id,
		)
		require.NoError(t, err)

		// The custom synthesizer should have been called
		assert.True(t, customSynthesizer.called)
	})
}

type mockPublicKeyIDSynthesizer struct {
	id     []byte
	called bool
}

func (m *mockPublicKeyIDSynthesizer) PublicKeyID(key any) ([]byte, error) {
	m.called = true
	return m.id, nil
}

func TestExtractPKI(t *testing.T) {
	t.Parallel()

	t.Run("extract with single extractor", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		// Add extractor with a valid ECDSA public key
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		extractor := &mockPublicKeyExtractor{
			key: &privateKey.PublicKey,
		}
		err = service.AddPublicKeyExtractor(extractor)
		require.NoError(t, err)

		id := []byte("test-id")
		pkiID := service.ExtractPKI(id)
		require.NotNil(t, pkiID)
		assert.True(t, extractor.called)
	})

	t.Run("extract with multiple extractors", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		// Add first extractor that returns nil
		extractor1 := &mockPublicKeyExtractor{
			key: nil,
			err: errors.New("not found"),
		}
		err = service.AddPublicKeyExtractor(extractor1)
		require.NoError(t, err)

		// Add second extractor that succeeds
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		extractor2 := &mockPublicKeyExtractor{
			key: &privateKey.PublicKey,
		}
		err = service.AddPublicKeyExtractor(extractor2)
		require.NoError(t, err)

		id := []byte("test-id")
		pkiID := service.ExtractPKI(id)
		require.NotNil(t, pkiID)
		assert.True(t, extractor1.called)
		assert.True(t, extractor2.called)
	})

	t.Run("no extractors returns nil", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id := []byte("test-id")
		pkiID := service.ExtractPKI(id)
		assert.Nil(t, pkiID)
	})

	t.Run("all extractors fail returns nil", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		extractor := &mockPublicKeyExtractor{
			key: nil,
			err: errors.New("extraction failed"),
		}
		err = service.AddPublicKeyExtractor(extractor)
		require.NoError(t, err)

		id := []byte("test-id")
		pkiID := service.ExtractPKI(id)
		assert.Nil(t, pkiID)
	})

	t.Run("nil extractor returns error", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		err = service.AddPublicKeyExtractor(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "should not be nil")
	})
}

type mockPublicKeyExtractor struct {
	key    any
	err    error
	called bool
}

func (m *mockPublicKeyExtractor) ExtractPublicKey(id view.Identity) (any, error) {
	m.called = true
	return m.key, m.err
}

func TestResolveIdentities(t *testing.T) {
	t.Parallel()
	t.Run("resolve single endpoint", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id := []byte("alice-id")
		_, err = service.AddResolver(
			"alice",
			"example.com",
			map[string]string{"p2p": "localhost:8080"},
			nil,
			id,
		)
		require.NoError(t, err)

		ids, err := service.ResolveIdentities("alice")
		require.NoError(t, err)
		require.Len(t, ids, 1)
		assert.Equal(t, view.Identity(id), ids[0])
	})

	t.Run("resolve multiple endpoints", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		aliceID := []byte("alice-id")
		bobID := []byte("bob-id")

		_, err = service.AddResolver("alice", "example.com", map[string]string{"p2p": "localhost:8080"}, nil, aliceID)
		require.NoError(t, err)
		_, err = service.AddResolver("bob", "example.com", map[string]string{"p2p": "localhost:8081"}, nil, bobID)
		require.NoError(t, err)

		ids, err := service.ResolveIdentities("alice", "bob")
		require.NoError(t, err)
		require.Len(t, ids, 2)
		assert.Equal(t, view.Identity(aliceID), ids[0])
		assert.Equal(t, view.Identity(bobID), ids[1])
	})

	t.Run("error when endpoint not found", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		_, err = service.ResolveIdentities("non-existent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot find the idnetity")
	})

	t.Run("error on first of multiple endpoints", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		bobID := []byte("bob-id")
		_, err = service.AddResolver("bob", "example.com", map[string]string{"p2p": "localhost:8081"}, nil, bobID)
		require.NoError(t, err)

		_, err = service.ResolveIdentities("non-existent", "bob")
		require.Error(t, err)
	})
}

func TestResolverMethods(t *testing.T) {
	t.Parallel()
	t.Run("GetName", func(t *testing.T) {
		t.Parallel()
		resolver := &endpoint.Resolver{
			ResolverInfo: endpoint.ResolverInfo{
				Name: "test-name",
			},
		}
		assert.Equal(t, "test-name", resolver.GetName())
	})

	t.Run("GetAddress", func(t *testing.T) {
		t.Parallel()
		resolver := &endpoint.Resolver{
			ResolverInfo: endpoint.ResolverInfo{
				Addresses: map[endpoint.PortName]string{
					endpoint.P2PPort:  "localhost:8080",
					endpoint.ViewPort: "localhost:8081",
				},
			},
		}
		assert.Equal(t, "localhost:8080", resolver.GetAddress(endpoint.P2PPort))
		assert.Equal(t, "localhost:8081", resolver.GetAddress(endpoint.ViewPort))
		assert.Equal(t, "", resolver.GetAddress(endpoint.ListenPort))
	})
}

func TestResolveWithBinding(t *testing.T) {
	t.Parallel()
	t.Run("resolve ephemeral identity through binding", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		longTermID := []byte("long-term-id")
		ephemeralID := []byte("ephemeral-id")

		// Add resolver for long-term identity
		_, err = service.AddResolver(
			"alice",
			"example.com",
			map[string]string{"p2p": "localhost:8080"},
			nil,
			longTermID,
		)
		require.NoError(t, err)

		// Setup binding store to return long-term ID for ephemeral ID
		bindingStore.GetLongTermReturns(longTermID, nil)

		// Resolve using ephemeral ID
		resolvedID, addresses, _, err := service.Resolve(context.Background(), ephemeralID)
		require.NoError(t, err)
		assert.Equal(t, view.Identity(longTermID), resolvedID)
		// localhost resolves to either IPv4 or IPv6
		assert.Contains(t, []string{"localhost:8080", "127.0.0.1:8080", "[::1]:8080"}, addresses[endpoint.P2PPort])

		// Verify binding store was called
		assert.Equal(t, 1, bindingStore.GetLongTermCallCount())
	})

	t.Run("resolve returns nil when identity not found", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.GetLongTermReturns([]byte("unknown-id"), nil)
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		_, _, _, err = service.Resolve(context.Background(), []byte("unknown-id"))
		require.Error(t, err)
		assert.ErrorIs(t, err, endpoint.ErrNotFound)
	})
}

func TestConcurrency(t *testing.T) {
	t.Parallel()
	t.Run("concurrent resolver additions", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.PutBindingsReturns(nil)
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		var wg sync.WaitGroup
		numGoroutines := 50

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				defer wg.Done()
				name := string(rune('a' + idx))
				id := []byte(name + "-id")
				_, err := service.AddResolver(
					name,
					"example.com",
					map[string]string{"p2p": "localhost:8080"},
					nil,
					id,
				)
				assert.NoError(t, err)
			}(i)
		}
		wg.Wait()

		resolvers := service.Resolvers()
		assert.Len(t, resolvers, numGoroutines)
	})

	t.Run("concurrent resolver updates", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id := []byte("test-id")
		_, err = service.AddResolver(
			"test",
			"example.com",
			map[string]string{"p2p": "localhost:8080"},
			nil,
			id,
		)
		require.NoError(t, err)

		var wg sync.WaitGroup
		numGoroutines := 50

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				defer wg.Done()
				_, err := service.UpdateResolver(
					"test",
					"example.com",
					map[string]string{"p2p": "localhost:9000"},
					nil,
					id,
				)
				assert.NoError(t, err)
			}(i)
		}
		wg.Wait()
	})

	t.Run("concurrent reads and writes", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.GetLongTermReturns(nil, nil)
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id := []byte("test-id")
		_, err = service.AddResolver(
			"test",
			"example.com",
			map[string]string{"p2p": "localhost:8080"},
			nil,
			id,
		)
		require.NoError(t, err)

		var wg sync.WaitGroup
		numGoroutines := 100

		// Half readers, half writers
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			if i%2 == 0 {
				// Reader
				go func() {
					defer wg.Done()
					_, err := service.GetResolver(context.Background(), id)
					assert.NoError(t, err)
				}()
			} else {
				// Writer
				go func() {
					defer wg.Done()
					_, err := service.UpdateResolver(
						"test",
						"example.com",
						map[string]string{"p2p": "localhost:9000"},
						nil,
						id,
					)
					assert.NoError(t, err)
				}()
			}
		}
		wg.Wait()
	})
}

func TestEdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("empty identity", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.GetLongTermReturns(nil, nil)
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		_, err = service.GetResolver(context.Background(), view.Identity(""))
		require.Error(t, err)
	})

	t.Run("nil identity", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		bindingStore.GetLongTermReturns(nil, nil)
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		_, err = service.GetResolver(context.Background(), nil)
		require.Error(t, err)
	})

	t.Run("resolver with empty addresses", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id := []byte("test-id")
		_, err = service.AddResolver(
			"test",
			"example.com",
			map[string]string{},
			nil,
			id,
		)
		require.NoError(t, err)

		resolver, err := service.GetResolver(context.Background(), id)
		require.NoError(t, err)
		assert.Empty(t, resolver.Addresses)
	})

	t.Run("resolver with nil aliases", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id := []byte("test-id")
		_, err = service.AddResolver(
			"test",
			"example.com",
			map[string]string{"p2p": "localhost:8080"},
			nil,
			id,
		)
		require.NoError(t, err)

		resolver, err := service.GetResolver(context.Background(), id)
		require.NoError(t, err)
		assert.Nil(t, resolver.Aliases)
	})

	t.Run("resolver with empty string aliases", func(t *testing.T) {
		t.Parallel()
		bindingStore := &mock.BindingStore{}
		service, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		id := []byte("test-id")
		_, err = service.AddResolver(
			"test",
			"example.com",
			map[string]string{"p2p": "localhost:8080"},
			[]string{"", "valid-alias", ""},
			id,
		)
		require.NoError(t, err)

		// Empty aliases should be ignored
		_, err = service.GetResolver(context.Background(), view.Identity(""))
		require.Error(t, err)

		// Valid alias should work
		resolver, err := service.GetResolver(context.Background(), view.Identity("valid-alias"))
		require.NoError(t, err)
		assert.Equal(t, id, resolver.ID)
	})
}
