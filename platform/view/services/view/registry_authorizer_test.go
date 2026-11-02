/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	viewapi "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// named returns an authorizer, a func and so not comparable, whose error is name.
func named(name string) view.AuthorizerFunc {
	return func(context.Context, viewapi.Identity) error { return errors.New(name) }
}

// responderLookup is implemented by view.Registry and view.Manager.
type responderLookup interface {
	ResponderForCaller(string) (viewapi.View, viewapi.Identity, view.ResponderAuthorizer, error)
}

// lookup returns the responder, identity and authorizer name ("" for none) registered for caller.
func lookup(t *testing.T, r responderLookup, caller string) (viewapi.View, viewapi.Identity, string) {
	t.Helper()
	v, id, a, err := r.ResponderForCaller(caller)
	require.NoError(t, err)
	if a == nil {
		return v, id, ""
	}
	return v, id, a.Authorize(t.Context(), nil).Error()
}

// One responder serves two initiators, each with its own identity and authorizer.
func TestRegistry_PerInitiatorOptions(t *testing.T) {
	t.Parallel()
	r := view.NewRegistry()
	resp := &mock.View{}
	require.NoError(t, r.RegisterResponderWithOptions(resp, "initA", view.WithIdentity(viewapi.Identity("idA")), view.WithAuthorizer(named("a"))))
	require.NoError(t, r.RegisterResponderWithOptions(resp, "initB", view.WithIdentity(viewapi.Identity("idB")), view.WithAuthorizer(named("b"))))

	v, id, a := lookup(t, r, "initA")
	require.Same(t, resp, v)
	require.Equal(t, viewapi.Identity("idA"), id)
	require.Equal(t, "a", a)
	v, id, a = lookup(t, r, "initB")
	require.Same(t, resp, v)
	require.Equal(t, viewapi.Identity("idB"), id)
	require.Equal(t, "b", a)
}

// A re-registration without an authorizer keeps the initiator's authorizer; one with an
// authorizer replaces it; the responder and identity are always replaced.
func TestRegistry_ReregistrationKeepsAuthorizer(t *testing.T) {
	t.Parallel()
	r := view.NewRegistry()
	first, second := &mock.View{}, &mock.View{}
	require.NoError(t, r.RegisterResponderWithOptions(first, "init", view.WithAuthorizer(named("a"))))

	require.NoError(t, r.RegisterResponderWithIdentity(second, viewapi.Identity("id2"), "init"))
	v, id, a := lookup(t, r, "init")
	require.Same(t, second, v)
	require.Equal(t, viewapi.Identity("id2"), id)
	require.Equal(t, "a", a)

	require.NoError(t, r.RegisterResponderWithOptions(first, "init", view.WithAuthorizer(named("b"))))
	v, id, a = lookup(t, r, "init")
	require.Same(t, first, v)
	require.True(t, id.IsNone(), "identity is replaced")
	require.Equal(t, "b", a)
}

// funcView is a view type that is not comparable.
type funcView func(viewapi.Context) (any, error)

func (f funcView) Call(c viewapi.Context) (any, error) { return f(c) }

// Registering a non-comparable responder again for an initiator does not panic, and a
// responder is not returned as an initiator view.
func TestRegistry_ReregistrationOfNonComparableView(t *testing.T) {
	t.Parallel()
	r := view.NewRegistry()
	resp := funcView(func(viewapi.Context) (any, error) { return nil, nil })
	for range 3 {
		require.NoError(t, r.RegisterResponder(resp, "init"))
	}
	v, err := r.GetResponder("init")
	require.NoError(t, err)
	require.NotNil(t, v)
	_, err = r.GetView(view.GetIdentifier(resp))
	require.ErrorIs(t, err, view.ErrViewNotFound)
}

func TestManager_RegisterResponderWithOptions(t *testing.T) {
	t.Parallel()
	m := view.NewManager(&mock.IdentityProvider{}, view.NewRegistry(), nil, nil, nil)
	resp := &mock.View{}
	require.NoError(t, m.RegisterResponderWithOptions(resp, "init", view.WithIdentity(viewapi.Identity("id")), view.WithAuthorizer(named("a"))))
	v, id, a := lookup(t, m, "init")
	require.Same(t, resp, v)
	require.Equal(t, viewapi.Identity("id"), id)
	require.Equal(t, "a", a)
	_, _, _, err := m.ResponderForCaller("other")
	require.ErrorIs(t, err, view.ErrResponderNotFound)
}
