/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	viewapi "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// pingInitiator declares the protocol pingpong/ping.
type pingInitiator struct{ mock.View }

func (*pingInitiator) Protocol() (component, name string) { return "pingpong", "ping" }

// movedPingInitiator is pingInitiator after it moved to another package.
type movedPingInitiator struct{ pingInitiator }

func TestRegistryProtocol(t *testing.T) {
	t.Parallel()
	r := view.NewRegistry()
	responder := &mock.View{}
	require.NoError(t, r.RegisterResponderWithOptions(responder, &pingInitiator{}, view.WithProtocol("pingpong", "ping")))

	moved := &movedPingInitiator{}
	got, _, _, err := r.ResponderForCaller("pingpong/ping", view.GetIdentifier(moved))
	require.NoError(t, err)
	require.Same(t, responder, got, "found by name after the initiator type moved")

	got, _, _, err = r.ResponderForCaller("", view.GetIdentifier(&pingInitiator{}))
	require.NoError(t, err)
	require.Same(t, responder, got, "an initiator without a name is still found by type path")

	got, _, _, err = r.ResponderForCaller("other/name", view.GetIdentifier(&pingInitiator{}))
	require.NoError(t, err)
	require.Same(t, responder, got, "an unknown name falls back to the type path")

	// When the name and the type path resolve to different responders, the name wins: it carries
	// the responder and authorizer of the protocol, which a stale type-path registration must not
	// shadow.
	byName, byType := &mock.View{}, &mock.View{}
	require.NoError(t, r.RegisterResponderWithOptions(byName, "app/X", view.WithProtocol("a", "b")))
	require.NoError(t, r.RegisterResponder(byType, "app/Y"))
	got, _, _, err = r.ResponderForCaller("a/b", "app/Y")
	require.NoError(t, err)
	require.Same(t, byName, got, "the protocol name wins over the type path")

	_, _, _, err = r.ResponderForCaller("pingpong/pong", "app/Unknown")
	require.ErrorIs(t, err, view.ErrResponderNotFound)

	// A type path that looks like a protocol name is not found as one.
	require.NoError(t, r.RegisterResponder(&mock.View{}, "main/initiator"))
	_, _, _, err = r.ResponderForCaller("main/initiator", "app/Unknown")
	require.ErrorIs(t, err, view.ErrResponderNotFound)

	// A later registration for the initiator without WithProtocol keeps the name.
	other := &mock.View{}
	require.NoError(t, r.RegisterResponder(other, &pingInitiator{}))
	got, _, _, err = r.ResponderForCaller("pingpong/ping", "")
	require.NoError(t, err)
	require.Same(t, other, got)

	for _, opt := range []view.ResponderOption{view.WithProtocol("Ping", "x"), view.WithProtocol("a/b", "c"), view.WithProtocol("", "x")} {
		require.ErrorContains(t, r.RegisterResponderWithOptions(&mock.View{}, "app/I", opt), "invalid protocol name")
	}
	require.Error(t, r.RegisterResponderWithOptions(&mock.View{}, "", view.WithProtocol("a", "b")), "a protocol needs an initiator")
}

// protocolSession records the protocol name GetSession sets.
type protocolSession struct {
	*mock.Session
	protocol string
}

func (s *protocolSession) SetProtocol(name string) { s.protocol = name }

// TestGetSessionSetsProtocol checks that a new session states the protocol of the view it is
// opened for, as bundled views do with Initiator() from a child context.
func TestGetSessionSetsProtocol(t *testing.T) {
	t.Parallel()
	resolver := &mock.EndpointService{}
	resolver.ResolverReturns(&endpoint.Resolver{ID: viewapi.Identity("bob")}, []byte("pkid"), nil)
	sessionFactory := &mock.SessionFactory{}
	ctx, err := view.NewContextForInitiator("c", context.Background(), view.NewServiceProvider(), sessionFactory, resolver, &mock.IdentityProvider{}, nil, &pingInitiator{}, emptyTracer, nil)
	require.NoError(t, err)
	child := view.NewChildContextFromParent(ctx)

	named := &protocolSession{Session: &mock.Session{}}
	sessionFactory.NewSessionReturns(named, nil)
	_, err = child.GetSession(child.Initiator(), viewapi.Identity("bob"))
	require.NoError(t, err)
	require.Equal(t, "pingpong/ping", named.protocol)

	for v, party := range map[viewapi.View]string{&mock.View{}: "carol", &badProtocol{}: "dave"} {
		unnamed := &protocolSession{Session: &mock.Session{}}
		sessionFactory.NewSessionReturns(unnamed, nil)
		_, err = child.GetSession(v, viewapi.Identity(party))
		require.NoError(t, err)
		require.Empty(t, unnamed.protocol, "no name, or an invalid one, is not stated")
	}
}

type badProtocol struct{ mock.View }

func (*badProtocol) Protocol() (component, name string) { return "a/b", "c" }

// TestRegistryProtocolReplacement checks that re-registering never leaves a name reaching a
// responder or authorizer it no longer should.
func TestRegistryProtocolReplacement(t *testing.T) {
	t.Parallel()
	deny := view.AuthorizerFunc(func(context.Context, viewapi.Identity) error { return view.ErrDenied })

	// The initiator moves to another name: the old name is released.
	r := view.NewRegistry()
	require.NoError(t, r.RegisterResponderWithOptions(&mock.View{}, "app/X", view.WithProtocol("a", "b")))
	require.NoError(t, r.RegisterResponderWithOptions(&mock.View{}, "app/X", view.WithProtocol("a", "c"), view.WithAuthorizer(deny)))
	_, _, _, err := r.ResponderForCaller("a/b", "")
	require.ErrorIs(t, err, view.ErrResponderNotFound, "the old name is gone")
	_, _, a, err := r.ResponderForCaller("a/c", "")
	require.NoError(t, err)
	require.NotNil(t, a)

	// Another initiator cannot take a name, and a default re-registration keeps the owner's.
	r = view.NewRegistry()
	owner := &mock.View{}
	require.NoError(t, r.RegisterResponderWithOptions(owner, "app/X", view.WithProtocol("a", "b"), view.WithAuthorizer(deny)))
	require.ErrorContains(t, r.RegisterResponderWithOptions(&mock.View{}, "app/Y", view.WithProtocol("a", "b")), "registered for another initiator")
	again := &mock.View{}
	require.NoError(t, r.RegisterResponder(again, "app/X"))
	got, _, a, err := r.ResponderForCaller("a/b", "")
	require.NoError(t, err)
	require.Same(t, again, got)
	require.NotNil(t, a, "the re-registration keeps the authorizer")
	_, _, _, err = r.ResponderForCaller("", "app/Y")
	require.ErrorIs(t, err, view.ErrResponderNotFound, "the refused registration left nothing behind")
}
