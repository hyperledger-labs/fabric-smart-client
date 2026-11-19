/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/security"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// A local identity resolves to its resolver and is bound to its identity, without a row in the
// binding store, and a row stored before, such as one to a rotated identity, does not shadow it.
func TestAddLocalIdentities_InMemoryAndShadowsStoredRow(t *testing.T) {
	t.Parallel()
	svc, store, p := newBindingTestService(t)
	alice, x, y := view.Identity("alice"), view.Identity("x"), view.Identity("y")
	require.NoError(t, store.PutBindings(t.Context(), view.Identity("alice-before-rotation"), y))

	require.NoError(t, svc.AddLocalIdentities(t.Context(), alice, x, y))

	for _, id := range []view.Identity{x, y} {
		requireResolvesTo(t, svc, id, "alice")
		assert.True(t, svc.IsBoundTo(t.Context(), id, alice), "IsBoundTo(%s, alice)", id)
	}
	lt, err := store.GetLongTerm(t.Context(), x)
	require.NoError(t, err)
	assert.True(t, lt.IsNone(), "x is stored")
	assert.Zero(t, p.count(security.KindBindingConflict))
}

// The identity of another resolver is not registered and counts a refusal, as with Bind.
func TestAddLocalIdentities_RefusesIdentityOfAnotherResolver(t *testing.T) {
	t.Parallel()
	svc, _, p := newBindingTestService(t)

	require.NoError(t, svc.AddLocalIdentities(t.Context(), view.Identity("alice"), view.Identity("bob")))

	requireResolvesTo(t, svc, view.Identity("bob"), "bob")
	assert.InDelta(t, 1, p.count(security.KindBindingRefused), 0)
}

// Without a resolver for longTerm, nothing is registered or stored.
func TestAddLocalIdentities_FailsWithoutResolver(t *testing.T) {
	t.Parallel()
	svc, store, _ := newBindingTestService(t)
	x := view.Identity("x")

	require.ErrorIs(t, svc.AddLocalIdentities(t.Context(), view.Identity("dave"), x), endpoint.ErrNotFound)

	_, err := svc.GetResolver(t.Context(), x)
	require.ErrorIs(t, err, endpoint.ErrNotFound)
	lt, err := store.GetLongTerm(t.Context(), x)
	require.NoError(t, err)
	assert.True(t, lt.IsNone())
}

// Local identities follow their resolver through UpdateResolver and go with RemoveResolver.
func TestAddLocalIdentities_UpdateAndRemoveResolver(t *testing.T) {
	t.Parallel()
	svc, _, _ := newBindingTestService(t)
	alice, x := view.Identity("alice"), view.Identity("x")
	require.NoError(t, svc.AddLocalIdentities(t.Context(), alice, x))

	_, err := svc.UpdateResolver("alice", "org1", map[string]string{string(endpoint.P2PPort): "alice:2"}, nil, alice)
	require.NoError(t, err)
	requireResolvesTo(t, svc, x, "alice")

	removed, err := svc.RemoveResolver(alice)
	require.NoError(t, err)
	require.True(t, removed)
	_, err = svc.GetResolver(t.Context(), x)
	require.ErrorIs(t, err, endpoint.ErrNotFound)
}

// IsBoundTo compares a local identity with a stored binding through the resolver's identity,
// and a resolver's name or address is not an identity of it.
func TestIsBoundTo_LocalIdentities(t *testing.T) {
	t.Parallel()
	svc, _, _ := newBindingTestService(t)
	alice, x, recipient := view.Identity("alice"), view.Identity("x"), view.Identity("alice-recipient")
	require.NoError(t, svc.AddLocalIdentities(t.Context(), alice, x))
	require.NoError(t, svc.BindProven(t.Context(), alice, recipient))
	_, err := svc.AddResolver("carol", "org1", map[string]string{string(endpoint.P2PPort): "carol:1"}, nil, view.Identity("carol-id"))
	require.NoError(t, err)

	assert.True(t, svc.IsBoundTo(t.Context(), x, recipient))
	assert.False(t, svc.IsBoundTo(t.Context(), x, view.Identity("bob")))
	assert.False(t, svc.IsBoundTo(t.Context(), view.Identity("carol"), view.Identity("carol-id")), "name")
	assert.False(t, svc.IsBoundTo(t.Context(), view.Identity("carol:1"), view.Identity("carol-id")), "address")
}

// A lookup that follows a binding to an identity no resolver has names the stale binding, and
// one of an identity without a resolver or a binding names the identity.
func TestGetResolver_StaleBinding(t *testing.T) {
	t.Parallel()
	svc, store, _ := newBindingTestService(t)
	recipient, old := view.Identity("alice-recipient"), view.Identity("alice-before-rotation")
	require.NoError(t, store.PutBindings(t.Context(), old, recipient))

	_, err := svc.GetResolver(t.Context(), recipient)
	require.ErrorIs(t, err, endpoint.ErrNotFound)
	require.ErrorContains(t, err, "["+recipient.String()+"] is bound to ["+old.String()+"], which no resolver has")

	_, err = svc.GetResolver(t.Context(), view.Identity("unknown"))
	require.ErrorIs(t, err, endpoint.ErrNotFound)
	require.ErrorContains(t, err, "endpoint not found for identity "+view.Identity("unknown").UniqueID())
}

// An identity another resolver already has stays with it, and the attempt counts a conflict,
// also after either resolver is updated.
func TestAddLocalIdentities_KeepsIdentityOfAnotherResolver(t *testing.T) {
	t.Parallel()
	svc, _, p := newBindingTestService(t)
	x := view.Identity("x")
	require.NoError(t, svc.AddLocalIdentities(t.Context(), view.Identity("alice"), x))

	require.NoError(t, svc.AddLocalIdentities(t.Context(), view.Identity("bob"), x))

	requireResolvesTo(t, svc, x, "alice")
	assert.InDelta(t, 1, p.count(security.KindBindingConflict), 0)
	for _, name := range []string{"bob", "alice"} {
		_, err := svc.UpdateResolver(name, "org1", map[string]string{string(endpoint.P2PPort): name + ":2"}, nil, view.Identity(name))
		require.NoError(t, err)
		requireResolvesTo(t, svc, x, "alice")
	}
}

// An entry that reuses a resolver's name attaches its identity and aliases to that resolver.
func TestAddResolver_NameMatchAttachesIdentityAndAliases(t *testing.T) {
	t.Parallel()
	svc, store, _ := newBindingTestService(t)
	fabricID := view.Identity("alice-fabric")

	rootID, err := svc.AddResolver("alice", "", nil, []string{"alice-fabric-alias"}, fabricID)
	require.NoError(t, err)

	require.Equal(t, view.Identity("alice"), rootID)
	for _, id := range []view.Identity{fabricID, view.Identity("alice-fabric-alias")} {
		requireResolvesTo(t, svc, id, "alice")
		assert.True(t, svc.IsBoundTo(t.Context(), id, view.Identity("alice")), "IsBoundTo(%s, alice)", id)
		lt, err := store.GetLongTerm(t.Context(), id)
		require.NoError(t, err)
		assert.True(t, lt.IsNone(), "%s is stored", id)
	}
}

// Bind to an identity of a resolver binds to the resolver's identity, and does not follow a
// row stored for that identity, such as one an older version wrote before a rotation.
func TestBind_ToLocalIdentityIgnoresStoredRow(t *testing.T) {
	t.Parallel()
	svc, store, _ := newBindingTestService(t)
	alice, m, wallet := view.Identity("alice"), view.Identity("alice-msp"), view.Identity("wallet")
	require.NoError(t, store.PutBindings(t.Context(), view.Identity("alice-before-rotation"), m))
	require.NoError(t, svc.AddLocalIdentities(t.Context(), alice, m))

	require.NoError(t, svc.Bind(t.Context(), m, wallet))

	requireResolvesTo(t, svc, wallet, "alice")
	assert.True(t, svc.IsBoundTo(t.Context(), wallet, alice))
	assert.True(t, svc.IsBoundTo(t.Context(), wallet, m))
}

// A row that binds an identity to an identity of a resolver, as an older version wrote, resolves
// to the resolver.
func TestIsBoundTo_StoredRowToLocalIdentity(t *testing.T) {
	t.Parallel()
	svc, store, _ := newBindingTestService(t)
	alice, m, wallet := view.Identity("alice"), view.Identity("alice-msp"), view.Identity("wallet")
	require.NoError(t, svc.AddLocalIdentities(t.Context(), alice, m))
	require.NoError(t, store.PutBindings(t.Context(), m, wallet))

	requireResolvesTo(t, svc, wallet, "alice")
	assert.True(t, svc.IsBoundTo(t.Context(), wallet, alice))
}

// An unproven bind of an identity the index holds for another resolver stores nothing and
// counts a conflict; one for the resolver it belongs to is dropped; a proven bind is refused.
func TestBind_IdentityOfAnotherResolver(t *testing.T) {
	t.Parallel()
	svc, store, p := newBindingTestService(t)
	x := view.Identity("alice-fabric")
	require.NoError(t, svc.AddLocalIdentities(t.Context(), view.Identity("alice"), x))

	require.NoError(t, svc.Bind(t.Context(), view.Identity("bob"), x))
	assert.InDelta(t, 1, p.count(security.KindBindingConflict), 0)
	require.NoError(t, svc.Bind(t.Context(), view.Identity("alice"), x))
	assert.InDelta(t, 1, p.count(security.KindBindingConflict), 0)
	lt, err := store.GetLongTerm(t.Context(), x)
	require.NoError(t, err)
	assert.True(t, lt.IsNone(), "x is stored")

	require.ErrorIs(t, svc.BindProven(t.Context(), view.Identity("bob"), x), driver.ErrBindingRefused)
	assert.InDelta(t, 2, p.count(security.KindBindingConflict), 0)
	requireResolvesTo(t, svc, x, "alice")
}

// A resolver that takes an identity another resolver registered keeps it when the other
// resolver is updated.
func TestUpdateResolver_KeepsIdentityAnotherResolverTook(t *testing.T) {
	t.Parallel()
	svc, _, _ := newBindingTestService(t)
	x := view.Identity("x")
	require.NoError(t, svc.AddLocalIdentities(t.Context(), view.Identity("alice"), x))
	_, err := svc.AddResolver("dave", "org1", map[string]string{string(endpoint.P2PPort): "dave:1"}, nil, x)
	require.NoError(t, err)
	requireResolvesTo(t, svc, x, "dave")

	_, err = svc.UpdateResolver("alice", "org1", map[string]string{string(endpoint.P2PPort): "alice:2"}, nil, view.Identity("alice"))
	require.NoError(t, err)
	requireResolvesTo(t, svc, x, "dave")
}

func requireResolvesTo(t *testing.T, svc *endpoint.Service, id view.Identity, name string) {
	t.Helper()
	r, err := svc.GetResolver(t.Context(), id)
	require.NoError(t, err, "GetResolver(%s)", id)
	require.Equal(t, name, r.Name, "GetResolver(%s)", id)
}
