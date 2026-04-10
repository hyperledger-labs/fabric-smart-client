/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/require"
)

func TestChildContext(t *testing.T) {
	t.Parallel()
	parent := &mock.MutableParentContext{}
	parent.IDReturns("parent-id")
	parent.MeReturns(view2.Identity("me"))
	parent.ContextReturns(context.Background())

	child := view.NewChildContextFromParent(parent)
	require.Equal(t, "parent-id", child.ID())
	require.Equal(t, view2.Identity("me"), child.Me())

	session := &mock.Session{}
	child2 := view.NewChildContextFromParentAndSession(parent, session)
	require.Equal(t, session, child2.Session())

	initiator := &mock.View{}
	child3 := view.NewChildContextFromParentAndInitiator(parent, initiator)
	require.Equal(t, initiator, child3.Initiator())

	child4 := view.NewChildContext(parent, session, initiator)
	require.Equal(t, session, child4.Session())
	require.Equal(t, initiator, child4.Initiator())

	// Test methods that delegate to parent
	child4.StartSpanFrom(context.Background(), "test")
	require.Equal(t, 1, parent.StartSpanFromCallCount())

	_, err := child4.GetService(reflect.TypeOf(""))
	require.NoError(t, err)
	require.Equal(t, 1, parent.GetServiceCallCount())

	err = child4.PutService("test")
	require.NoError(t, err)
	require.Equal(t, 1, parent.PutServiceCallCount())

	child4.IsMe(view2.Identity("me"))
	require.Equal(t, 1, parent.IsMeCallCount())

	_, err = child4.GetSession(nil, view2.Identity("party"))
	require.NoError(t, err)
	require.Equal(t, 1, parent.GetSessionCallCount())

	_, err = child4.GetSessionByID("sid", view2.Identity("party"))
	require.NoError(t, err)
	require.Equal(t, 1, parent.GetSessionByIDCallCount())

	child4.Context()
	require.Equal(t, 1, parent.ContextCallCount())

	err = child4.ResetSessions()
	require.NoError(t, err)
	require.Equal(t, 1, parent.ResetSessionsCallCount())

	called := false
	child4.OnError(func() { called = true })
	child4.Cleanup()
	require.True(t, called)

	child4.Dispose()
	require.Equal(t, 1, parent.DisposeCallCount())

	err = child4.PutSession(nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 1, parent.PutSessionCallCount())

	err = child4.PutSessionByID("", nil, nil)
	require.NoError(t, err)
	require.Equal(t, 1, parent.PutSessionByIDCallCount())

	// Test case where parent is not mutable
	parentNotMutable := &mock.ParentContext{}
	childNotMutable := view.NewChildContextFromParent(parentNotMutable)
	err = childNotMutable.PutService("test")
	require.NoError(t, err)
	err = childNotMutable.ResetSessions()
	require.NoError(t, err)

	// Test Initiator when w.initiator is nil
	parent.InitiatorReturns(initiator)
	childNoInitiator := view.NewChildContextFromParent(parent)
	require.Equal(t, initiator, childNoInitiator.Initiator())

	// Test Session when w.session is nil
	parent.SessionReturns(session)
	childNoSession := view.NewChildContextFromParent(parent)
	require.Equal(t, session, childNoSession.Session())

	// Test safeInvoke panic
	child.Cleanup() // no error funcs
	child.OnError(func() { panic("boom") })
	child.Cleanup() // should not panic
}
