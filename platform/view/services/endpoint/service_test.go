/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/security"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	memory "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
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
	require.Equal(t, map[endpoint.PortName]string{endpoint.P2PPort: "localhost:1010"}, resolvers[0].Addresses)
	id, err := service.GetIdentity("localhost:2020", nil)
	require.NoError(t, err)
	require.Equal(t, view.Identity("second_id"), id)
}

// TestResolversRacesWithPkiResolve copies a resolver's PKI while PkiResolve writes it.
// Run with -race.
func TestResolversRacesWithPkiResolve(t *testing.T) {
	t.Parallel()
	service, err := endpoint.NewService(&mock.BindingStore{})
	require.NoError(t, err)

	// no extractor yet, so AddResolver leaves the PKI empty and Resolve computes it later
	aliceID := view.Identity("alice_id")
	_, err = service.AddResolver("alice", "org1", map[string]string{string(endpoint.P2PPort): "localhost:1010"}, nil, aliceID)
	require.NoError(t, err)
	extractor := &mock.PublicKeyExtractor{}
	extractor.ExtractPublicKeyReturns([]byte("alice_pk"), nil)
	require.NoError(t, service.AddPublicKeyExtractor(extractor))

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _, _ = service.Resolve(t.Context(), aliceID)
	}()
	for range 100 {
		_ = service.Resolvers()
	}
	<-done
}

// AddResolver and UpdateResolver reject an address key that names no PortName, such as a
// misspelled P2P key, and two keys that differ only in case. Known keys are matched
// case-insensitively.
func TestAddResolver_RejectsUnknownAddressKey(t *testing.T) {
	t.Parallel()
	service, err := endpoint.NewService(&mock.BindingStore{})
	require.NoError(t, err)

	_, err = service.AddResolver("bob", "org1", map[string]string{"P2PAdress": "localhost:1010"}, nil, []byte("bob_id"))
	require.ErrorContains(t, err, "failed adding resolver [bob]: unknown address key [P2PAdress]")
	require.Empty(t, service.Resolvers())

	_, err = service.AddResolver("bob", "org1", map[string]string{"P2P": "localhost:1010", "p2p": "localhost:2020"}, nil, []byte("bob_id"))
	require.ErrorContains(t, err, "failed adding resolver [bob]: duplicate address key")
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

// countingProvider counts every Add on its counters by label values.
type countingProvider struct {
	metrics.Provider
	mu     sync.Mutex
	counts map[string]float64
}

func (p *countingProvider) NewCounter(metrics.CounterOpts) metrics.Counter {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.counts = map[string]float64{}
	return &countingCounter{p: p}
}

func (p *countingProvider) count(kind security.Kind) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.counts[string(kind)]
}

type countingCounter struct {
	p     *countingProvider
	label string
}

func (c *countingCounter) With(labelValues ...string) metrics.Counter {
	return &countingCounter{p: c.p, label: labelValues[len(labelValues)-1]}
}

func (c *countingCounter) Add(delta float64) {
	c.p.mu.Lock()
	defer c.p.mu.Unlock()
	c.p.counts[c.label] += delta
}

// newBindingTestService returns a service over an in-memory binding store with resolvers for
// alice and bob, and the provider that counts its rejections.
func newBindingTestService(t *testing.T) (*endpoint.Service, driver.BindingStore, *countingProvider) {
	t.Helper()
	store, err := memory.NewDriver().NewBinding("", t.Name())
	require.NoError(t, err)
	svc, err := endpoint.NewService(store)
	require.NoError(t, err)
	p := &countingProvider{}
	svc.SetRejections(security.NewRejections(p))

	for _, name := range []string{"alice", "bob"} {
		_, err = svc.AddResolver(name, "org1", map[string]string{string(endpoint.P2PPort): name + ":1"}, nil, view.Identity(name))
		require.NoError(t, err)
	}
	return svc, store, p
}

// An unproven bind of an ephemeral that is already bound to another party keeps the existing
// binding, returns nil and counts a conflict.
func TestBind_ConflictKeepsOwnerAndCounts(t *testing.T) {
	t.Parallel()
	svc, _, p := newBindingTestService(t)
	alice, bob, x := view.Identity("alice"), view.Identity("bob"), view.Identity("alice-recipient-x")

	require.NoError(t, svc.Bind(t.Context(), bob, x))
	require.NoError(t, svc.Bind(t.Context(), alice, x))

	resolved, _, _, err := svc.Resolve(t.Context(), x)
	require.NoError(t, err)
	require.Equal(t, bob, resolved)
	require.InDelta(t, 1, p.count(security.KindBindingConflict), 0)

	// binding again to the owner is not a conflict
	require.NoError(t, svc.Bind(t.Context(), bob, x))
	require.InDelta(t, 1, p.count(security.KindBindingConflict), 0)
}

// Bind does not store a resolver's identity as the ephemeral of another party, returns nil and
// counts a refusal.
func TestBind_RefusesResolverIdentityAsEphemeral(t *testing.T) {
	t.Parallel()
	svc, store, p := newBindingTestService(t)
	alice := view.Identity("alice")

	require.NoError(t, svc.Bind(t.Context(), view.Identity("mallory"), alice))

	lt, err := store.GetLongTerm(t.Context(), alice)
	require.NoError(t, err)
	require.Empty(t, lt)
	require.InDelta(t, 1, p.count(security.KindBindingRefused), 0)
}

// Bind does not store an identity with a self-row as the ephemeral of another party, returns nil
// and counts a refusal, while still binding the other ephemerals of the call.
func TestBind_RefusesSelfRowAsEphemeral(t *testing.T) {
	t.Parallel()
	svc, store, p := newBindingTestService(t)
	carol, mallory := view.Identity("carol"), view.Identity("mallory")
	require.NoError(t, svc.Bind(t.Context(), carol, view.Identity("carol-eph")))

	require.NoError(t, svc.Bind(t.Context(), mallory, carol, view.Identity("mallory-eph")))

	lt, err := store.GetLongTerm(t.Context(), carol)
	require.NoError(t, err)
	require.Equal(t, carol, lt)
	lt, err = store.GetLongTerm(t.Context(), view.Identity("mallory-eph"))
	require.NoError(t, err)
	require.Equal(t, mallory, lt)
	require.InDelta(t, 1, p.count(security.KindBindingRefused), 0)
	require.InDelta(t, 0, p.count(security.KindBindingConflict), 0)
}

// Bind counts a refusal for each ephemeral the store refuses, returns nil, and writes no row
// binding it to longTerm.
func TestBind_StoreRefusalCountedPerEphemeral(t *testing.T) {
	t.Parallel()
	svc, store, p := newBindingTestService(t)
	carol, dave, mallory := view.Identity("carol"), view.Identity("dave"), view.Identity("mallory")
	require.NoError(t, svc.Bind(t.Context(), carol, view.Identity("carol-eph")))
	require.NoError(t, svc.Bind(t.Context(), dave, view.Identity("dave-eph")))

	require.NoError(t, svc.Bind(t.Context(), mallory, carol, dave))

	for _, id := range []view.Identity{carol, dave} {
		lt, err := store.GetLongTerm(t.Context(), id)
		require.NoError(t, err)
		require.Equal(t, id, lt)
	}
	require.InDelta(t, 2, p.count(security.KindBindingRefused), 0)
}

// Concurrent AddResolver calls for the same name all return: the call that finds the name
// registered binds its id without holding the resolvers lock.
func TestAddResolver_ConcurrentSameNameDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	store, err := memory.NewDriver().NewBinding("", t.Name())
	require.NoError(t, err)
	svc, err := endpoint.NewService(store)
	require.NoError(t, err)

	const n = 16
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			_, err := svc.AddResolver("node", "org1", nil, nil, view.Identity(fmt.Sprintf("node-id-%d", i)))
			assert.NoError(t, err)
		})
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent AddResolver calls for the same name did not return")
	}
	require.Len(t, svc.Resolvers(), 1)
}

func TestBindProven_RefusesResolverIdentityAsEphemeral(t *testing.T) {
	t.Parallel()
	svc, store, p := newBindingTestService(t)
	alice := view.Identity("alice")

	err := svc.BindProven(t.Context(), view.Identity("mallory"), alice)
	require.ErrorIs(t, err, driver.ErrBindingRefused)

	lt, err := store.GetLongTerm(t.Context(), alice)
	require.NoError(t, err)
	require.Empty(t, lt)
	require.InDelta(t, 1, p.count(security.KindBindingRefused), 0)
}

func TestBindProven_RefusesSelfRowAsEphemeral(t *testing.T) {
	t.Parallel()
	svc, _, _ := newBindingTestService(t)
	carol := view.Identity("carol")
	require.NoError(t, svc.Bind(t.Context(), carol, view.Identity("carol-eph")))

	err := svc.BindProven(t.Context(), view.Identity("mallory"), carol)
	require.ErrorIs(t, err, driver.ErrBindingRefused)
}

// A proven bind replaces a binding another party planted for the ephemeral.
func TestBindProven_RepairsPlantedBinding(t *testing.T) {
	t.Parallel()
	svc, _, _ := newBindingTestService(t)
	alice, bob, x := view.Identity("alice"), view.Identity("bob"), view.Identity("alice-recipient-x")
	require.NoError(t, svc.Bind(t.Context(), bob, x))

	require.NoError(t, svc.BindProven(t.Context(), alice, x))

	resolved, _, _, err := svc.Resolve(t.Context(), x)
	require.NoError(t, err)
	require.Equal(t, alice, resolved)
}

// A proven bind stores the ephemeral under the given long term even if a planted row binds that
// long term to another party.
func TestBindProven_DoesNotFollowPlantedRedirect(t *testing.T) {
	t.Parallel()
	store, err := memory.NewDriver().NewBinding("", t.Name())
	require.NoError(t, err)
	svc, err := endpoint.NewService(store)
	require.NoError(t, err)
	alice, mallory, x := view.Identity("alice"), view.Identity("mallory"), view.Identity("x")
	require.NoError(t, svc.Bind(t.Context(), mallory, alice)) // alice is not known yet

	require.NoError(t, svc.BindProven(t.Context(), alice, x))
	lt, err := store.GetLongTerm(t.Context(), x)
	require.NoError(t, err)
	require.Equal(t, alice, lt)
}
