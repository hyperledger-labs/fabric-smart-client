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
	for range 100 {
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

// An empty label is not looked up, so a resolver registered without a name cannot
// claim callers identified by pkID alone.
func TestGetIdentity_EmptyLabelResolvesByPKID(t *testing.T) {
	t.Parallel()
	service, err := endpoint.NewService(&mock.BindingStore{})
	require.NoError(t, err)

	_, err = service.AddResolver("", "org1", nil, nil, []byte("unnamed_id"))
	require.NoError(t, err)
	_, err = service.AddResolver("bob", "org2", nil, nil, []byte("bob_id"))
	require.NoError(t, err)

	id, err := service.GetIdentity("", []byte("bob_id"))
	require.NoError(t, err)
	require.Equal(t, view.Identity("bob_id"), id)

	_, err = service.GetIdentity("", []byte("unknown"))
	require.ErrorIs(t, err, endpoint.ErrNotFound)
}

// A remote peer chooses its source address but not its verified pkID, so the pkID wins over
// a label that names another peer, and a pkID that resolves to nothing never falls back to it.
func TestGetIdentity_PKIDWinsOverLabel(t *testing.T) {
	t.Parallel()
	service, err := endpoint.NewService(&mock.BindingStore{})
	require.NoError(t, err)

	victimAddr := "127.0.0.1:20001"
	_, err = service.AddResolver("victim", "org1", map[string]string{"P2P": victimAddr}, nil, []byte("victim_id"))
	require.NoError(t, err)
	_, err = service.AddResolver("attacker", "org2", map[string]string{"P2P": "127.0.0.1:20002"}, nil, []byte("attacker_id"))
	require.NoError(t, err)

	for _, label := range []string{victimAddr, "victim", "127.0.0.1:54321"} {
		id, err := service.GetIdentity(label, []byte("attacker_id"))
		require.NoError(t, err)
		require.Equal(t, view.Identity("attacker_id"), id, label)
	}

	_, err = service.GetIdentity(victimAddr, []byte("unknown"))
	require.ErrorIs(t, err, endpoint.ErrNotFound)

	id, err := service.GetIdentity(victimAddr, nil)
	require.NoError(t, err)
	require.Equal(t, view.Identity("victim_id"), id)
}

// A resolver without a name is not indexed under the empty key or under "."+domain, so
// an empty label, pkID or identity matches no resolver. The resolver stays reachable by
// its ID and addresses.
func TestUnnamedResolver_NotIndexedUnderEmptyKey(t *testing.T) {
	t.Parallel()
	service, err := endpoint.NewService(&mock.BindingStore{})
	require.NoError(t, err)

	_, err = service.AddResolver("", "org1", map[string]string{string(endpoint.P2PPort): "localhost:1010"}, nil, []byte("unnamed_id"))
	require.NoError(t, err)

	for _, label := range []string{"", ".org1"} {
		_, err = service.GetIdentity(label, nil)
		require.ErrorIs(t, err, endpoint.ErrNotFound, "label [%s]", label)
	}
	_, _, _, err = service.Resolve(t.Context(), nil)
	require.ErrorIs(t, err, endpoint.ErrNotFound)
	_, err = service.RemoveResolver(nil)
	require.ErrorIs(t, err, endpoint.ErrNotFound)

	id, err := service.GetIdentity("", []byte("unnamed_id"))
	require.NoError(t, err)
	require.Equal(t, view.Identity("unnamed_id"), id)
	id, err = service.GetIdentity("localhost:1010", nil)
	require.NoError(t, err)
	require.Equal(t, view.Identity("unnamed_id"), id)
}

// Resolvers are deduplicated by name only when they have one: two resolvers without a
// name are registered separately and are not bound to each other.
func TestAddResolver_UnnamedResolversAreNotBound(t *testing.T) {
	t.Parallel()
	bindingStore := &mock.BindingStore{}
	service, err := endpoint.NewService(bindingStore)
	require.NoError(t, err)

	_, err = service.AddResolver("", "org1", map[string]string{string(endpoint.P2PPort): "localhost:1010"}, nil, []byte("first_id"))
	require.NoError(t, err)
	existing, err := service.AddResolver("", "org2", map[string]string{string(endpoint.P2PPort): "localhost:2020"}, nil, []byte("second_id"))
	require.NoError(t, err)
	require.Nil(t, existing)
	require.Zero(t, bindingStore.PutBindingsCallCount())
	require.Len(t, service.Resolvers(), 2)

	for _, tc := range []struct{ id, addr string }{{"first_id", "localhost:1010"}, {"second_id", "localhost:2020"}} {
		id, err := service.GetIdentity("", []byte(tc.id))
		require.NoError(t, err)
		require.Equal(t, view.Identity(tc.id), id)
		id, err = service.GetIdentity(tc.addr, nil)
		require.NoError(t, err)
		require.Equal(t, view.Identity(tc.id), id)
	}
}

// UpdateResolver with an empty name adds a resolver and leaves existing unnamed
// resolvers unchanged.
func TestUpdateResolver_EmptyNameAddsResolver(t *testing.T) {
	t.Parallel()
	service, err := endpoint.NewService(&mock.BindingStore{})
	require.NoError(t, err)

	_, err = service.AddResolver("", "org1", map[string]string{string(endpoint.P2PPort): "localhost:1010"}, nil, []byte("first_id"))
	require.NoError(t, err)
	_, err = service.UpdateResolver("", "org2", map[string]string{string(endpoint.P2PPort): "localhost:2020"}, nil, []byte("second_id"))
	require.NoError(t, err)

	resolvers := service.Resolvers()
	require.Len(t, resolvers, 2)
	require.Equal(t, []string{"localhost:1010"}, resolvers[0].AddressList)
	id, err := service.GetIdentity("localhost:2020", nil)
	require.NoError(t, err)
	require.Equal(t, view.Identity("second_id"), id)
}

// An address key that names no PortName, such as a misspelled P2P key, is rejected by
// AddResolver and UpdateResolver instead of storing the address under the empty PortName.
// Known keys are matched case-insensitively.
func TestAddResolver_RejectsUnknownAddressKey(t *testing.T) {
	t.Parallel()
	service, err := endpoint.NewService(&mock.BindingStore{})
	require.NoError(t, err)

	_, err = service.AddResolver("bob", "org1", map[string]string{"P2PAdress": "localhost:1010"}, nil, []byte("bob_id"))
	require.ErrorContains(t, err, "failed adding resolver [bob]: unknown address key [P2PAdress]")
	require.Empty(t, service.Resolvers())

	_, err = service.AddResolver("bob", "org1", map[string]string{"p2p": "localhost:1010"}, nil, []byte("bob_id"))
	require.NoError(t, err)
	_, ports, _, err := service.Resolve(t.Context(), []byte("bob_id"))
	require.NoError(t, err)
	require.Equal(t, map[endpoint.PortName]string{endpoint.P2PPort: "localhost:1010"}, ports)

	_, err = service.UpdateResolver("bob", "", map[string]string{"P2PAdress": "localhost:2020"}, nil, nil)
	require.ErrorContains(t, err, "failed updating resolver [bob]: unknown address key [P2PAdress]")
	_, ports, _, err = service.Resolve(t.Context(), []byte("bob_id"))
	require.NoError(t, err)
	require.Equal(t, map[endpoint.PortName]string{endpoint.P2PPort: "localhost:1010"}, ports)
}
